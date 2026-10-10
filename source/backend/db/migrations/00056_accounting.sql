-- +goose Up

-- Akuntansi SIAK fase A (FR-ACC-01/04/05; docs/ARCHITECTURE.md §4b): bagan akun (COA), periode bulanan, jurnal (draf → posting),
-- agregat saldo per periode. Jurnal terposting immutable: dijaga DB lewat policy RLS RESTRICTIVE (UPDATE/DELETE hanya baris draf) dan
-- tanpa hak UPDATE pada journal_lines; koreksi = jurnal balik (reverses_id). Aturan seimbang/periode ada di service Go.

CREATE TABLE accounts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL,
    parent_id   uuid,
    code        text        NOT NULL CHECK (char_length(code) BETWEEN 1 AND 32),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    kind        text        NOT NULL CHECK (kind IN ('group', 'ledger')),
    class       text        NOT NULL CHECK (class IN ('asset', 'liability', 'equity', 'revenue', 'cogs', 'expense')),
    normal_side text        NOT NULL CHECK (normal_side IN ('debit', 'credit')),
    is_cash_bank boolean    NOT NULL DEFAULT false,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT accounts_code_key UNIQUE (tenant_id, code),
    CONSTRAINT accounts_parent_fk FOREIGN KEY (tenant_id, parent_id) REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT accounts_not_self_parent CHECK (parent_id IS NULL OR parent_id <> id),
    -- Saldo normal diturunkan dari kelas akun.
    CONSTRAINT accounts_normal_side_chk CHECK (normal_side = CASE WHEN class IN ('asset', 'cogs', 'expense') THEN 'debit' ELSE 'credit' END),
    -- Akun kas/bank hanya akun buku (ledger) kelas aset.
    CONSTRAINT accounts_cash_bank_chk CHECK (NOT is_cash_bank OR (kind = 'ledger' AND class = 'asset'))
);
CREATE INDEX accounts_parent_idx ON accounts (tenant_id, parent_id);

-- Periode = satu bulan kalender (dibuat otomatis saat pertama dipakai). Tutup buku mengunci periode.
CREATE TABLE accounting_periods (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL,
    start_date  date        NOT NULL,
    end_date    date        NOT NULL,
    status      text        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    closed_at   timestamptz,
    closed_by   uuid,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounting_periods_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT accounting_periods_start_key UNIQUE (tenant_id, start_date),
    CONSTRAINT accounting_periods_month_chk CHECK (
        start_date = date_trunc('month', start_date)::date AND end_date = (start_date + interval '1 month' - interval '1 day')::date),
    CONSTRAINT accounting_periods_closer_fk FOREIGN KEY (tenant_id, closed_by) REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT accounting_periods_state_chk CHECK (
        (status = 'open' AND closed_at IS NULL AND closed_by IS NULL) OR (status = 'closed' AND closed_at IS NOT NULL AND closed_by IS NOT NULL))
);

-- Nomor dokumen jurnal: per jenis per tahun (JU/2026/000001), diberikan saat posting (draf tidak menghabiskan nomor).
CREATE TABLE journal_counters (
    tenant_id uuid   NOT NULL,
    type      text   NOT NULL,
    year      int    NOT NULL,
    last_no   bigint NOT NULL,
    PRIMARY KEY (tenant_id, type, year)
);

CREATE TABLE journal_entries (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL,
    outlet_id   uuid        NOT NULL,
    doc_no      text        CHECK (doc_no IS NULL OR char_length(doc_no) BETWEEN 1 AND 64),
    entry_date  date        NOT NULL,
    type        text        NOT NULL CHECK (type IN ('JU', 'KM', 'KK', 'TK', 'SALES', 'PURCHASE', 'AR', 'AP', 'OPENING')),
    status      text        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'posted')),
    narration   text        NOT NULL DEFAULT '' CHECK (char_length(narration) <= 500),
    source_type text        CHECK (source_type IS NULL OR char_length(source_type) BETWEEN 1 AND 40),
    source_ref  text        CHECK (source_ref IS NULL OR char_length(source_ref) BETWEEN 1 AND 100),
    reverses_id uuid,
    created_by  uuid        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    posted_by   uuid,
    posted_at   timestamptz,
    CONSTRAINT journal_entries_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT journal_entries_tenant_id_id_date_key UNIQUE (tenant_id, id, entry_date),
    CONSTRAINT journal_entries_doc_key UNIQUE (tenant_id, doc_no),
    CONSTRAINT journal_entries_outlet_fk   FOREIGN KEY (tenant_id, outlet_id)   REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT journal_entries_creator_fk  FOREIGN KEY (tenant_id, created_by)  REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT journal_entries_poster_fk   FOREIGN KEY (tenant_id, posted_by)   REFERENCES users (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT journal_entries_reverses_fk FOREIGN KEY (tenant_id, reverses_id) REFERENCES journal_entries (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT journal_entries_source_chk CHECK ((source_type IS NULL) = (source_ref IS NULL)),
    CONSTRAINT journal_entries_state_chk CHECK (
        (status = 'draft' AND doc_no IS NULL AND posted_by IS NULL AND posted_at IS NULL)
        OR (status = 'posted' AND doc_no IS NOT NULL AND posted_by IS NOT NULL AND posted_at IS NOT NULL))
);
-- Idempotensi jurnal otomatis (fase C) dan satu jurnal balik per jurnal.
CREATE UNIQUE INDEX journal_entries_source_key  ON journal_entries (tenant_id, source_type, source_ref) WHERE source_type IS NOT NULL;
CREATE UNIQUE INDEX journal_entries_reverses_key ON journal_entries (tenant_id, reverses_id) WHERE reverses_id IS NOT NULL;
CREATE INDEX journal_entries_date_idx ON journal_entries (tenant_id, entry_date DESC, id DESC);
CREATE INDEX journal_entries_draft_idx ON journal_entries (tenant_id, entry_date) WHERE status = 'draft';

CREATE TABLE journal_lines (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid          NOT NULL,
    entry_id   uuid          NOT NULL,
    seq        bigint        GENERATED ALWAYS AS IDENTITY,  -- urutan pencatatan (buku besar: urut dalam satu hari, kunci keyset)
    entry_date date          NOT NULL,                 -- salinan tanggal header (buku besar keyset tanpa JOIN); ikut berubah lewat ON UPDATE CASCADE
    line_no    int           NOT NULL CHECK (line_no >= 1),
    account_id uuid          NOT NULL,
    debit      numeric(18,2) NOT NULL DEFAULT 0 CHECK (debit >= 0 AND debit < 1e15),
    credit     numeric(18,2) NOT NULL DEFAULT 0 CHECK (credit >= 0 AND credit < 1e15),
    memo       text          NOT NULL DEFAULT '' CHECK (char_length(memo) <= 200),
    CONSTRAINT journal_lines_entry_line_key UNIQUE (tenant_id, entry_id, line_no),
    CONSTRAINT journal_lines_entry_fk FOREIGN KEY (tenant_id, entry_id, entry_date) REFERENCES journal_entries (tenant_id, id, entry_date)
        ON UPDATE CASCADE ON DELETE CASCADE,
    CONSTRAINT journal_lines_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT journal_lines_side_chk CHECK ((debit = 0) <> (credit = 0))
);
-- Buku besar: per akun, urut tanggal + seq (keyset).
CREATE UNIQUE INDEX journal_lines_ledger_idx ON journal_lines (tenant_id, account_id, entry_date, seq);

-- Agregat saldo per outlet/akun/bulan, diperbarui di transaksi posting yang sama.
CREATE TABLE account_period_balances (
    tenant_id    uuid          NOT NULL,
    outlet_id    uuid          NOT NULL,
    account_id   uuid          NOT NULL,
    period_month date          NOT NULL CHECK (period_month = date_trunc('month', period_month)::date),
    debit        numeric(18,2) NOT NULL DEFAULT 0,
    credit       numeric(18,2) NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, outlet_id, account_id, period_month),
    CONSTRAINT account_period_balances_outlet_fk  FOREIGN KEY (tenant_id, outlet_id)  REFERENCES outlets (tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT account_period_balances_account_fk FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX account_period_balances_account_idx ON account_period_balances (tenant_id, account_id, period_month);

ALTER TABLE accounts                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounting_periods       ENABLE ROW LEVEL SECURITY;
ALTER TABLE journal_counters         ENABLE ROW LEVEL SECURITY;
ALTER TABLE journal_entries          ENABLE ROW LEVEL SECURITY;
ALTER TABLE journal_lines            ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_period_balances  ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON accounts                USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON accounting_periods      USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON journal_counters        USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON journal_entries         USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON journal_lines           USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON account_period_balances USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Jurnal terposting immutable (lapis DB): UPDATE/DELETE header hanya untuk draf; baris jurnal hanya boleh ditambah/dihapus
-- selama header masih draf, dan tidak pernah di-UPDATE.
CREATE POLICY draft_only_update ON journal_entries AS RESTRICTIVE FOR UPDATE USING (status = 'draft') WITH CHECK (true);  -- WITH CHECK (true): draf boleh menjadi posted
CREATE POLICY draft_only_delete ON journal_entries AS RESTRICTIVE FOR DELETE USING (status = 'draft');
CREATE POLICY draft_only_insert ON journal_lines AS RESTRICTIVE FOR INSERT
    WITH CHECK (EXISTS (SELECT 1 FROM journal_entries e WHERE e.tenant_id = journal_lines.tenant_id AND e.id = journal_lines.entry_id AND e.status = 'draft'));
CREATE POLICY draft_only_delete ON journal_lines AS RESTRICTIVE FOR DELETE
    USING (EXISTS (SELECT 1 FROM journal_entries e WHERE e.tenant_id = journal_lines.tenant_id AND e.id = journal_lines.entry_id AND e.status = 'draft'));

REVOKE DELETE, TRUNCATE ON journal_counters, account_period_balances, accounting_periods FROM aciraba_app;
REVOKE UPDATE, TRUNCATE ON journal_lines FROM aciraba_app;
REVOKE TRUNCATE ON accounts, journal_entries FROM aciraba_app;
-- Header jurnal: hanya kolom yang sah berubah (tanggal/outlet/narasi saat draf; status, nomor, poster saat posting).
REVOKE UPDATE ON journal_entries FROM aciraba_app;
GRANT UPDATE (outlet_id, entry_date, narration, status, doc_no, posted_by, posted_at) ON journal_entries TO aciraba_app;
-- Periode: hanya status tutup/buka yang berubah.
REVOKE UPDATE ON accounting_periods FROM aciraba_app;
GRANT UPDATE (status, closed_at, closed_by) ON accounting_periods TO aciraba_app;

-- +goose Down
DROP TABLE account_period_balances;
DROP TABLE journal_lines;
DROP TABLE journal_entries;
DROP TABLE journal_counters;
DROP TABLE accounting_periods;
DROP TABLE accounts;
