-- +goose Up

-- Member/pelanggan + poin (Fase 3.3). Poin = ledger append-only (`member_point_movements`); saldo `members.points`
-- diubah di transaksi yang sama dengan movement-nya (pola sama dengan stok). Level member ditentukan OTOMATIS dari
-- `lifetime_points` (total poin yang pernah diperoleh; tidak berkurang oleh penukaran) — tidak disimpan, dihitung saat dibaca.

CREATE TABLE member_levels (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name            text          NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    min_points      int           NOT NULL CHECK (min_points >= 0),            -- ambang lifetime_points; 0 = level dasar
    spend_per_point numeric(18,2) NOT NULL CHECK (spend_per_point >= 0),       -- Rp belanja per 1 poin; 0 = tidak dapat poin
    point_value     numeric(18,2) NOT NULL CHECK (point_value >= 0),           -- Rp potongan per 1 poin saat ditukar
    active          boolean       NOT NULL DEFAULT true,
    created_at      timestamptz   NOT NULL DEFAULT now(),
    updated_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT member_levels_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT member_levels_min_points_key UNIQUE (tenant_id, min_points)
);
CREATE UNIQUE INDEX member_levels_name_key ON member_levels (tenant_id, lower(name));

CREATE TABLE member_counters (
    tenant_id uuid   PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    last_no   bigint NOT NULL
);

CREATE TABLE members (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid          NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    code            text          NOT NULL CHECK (code ~ '^[A-Za-z0-9][A-Za-z0-9._/-]{0,39}$'),
    name            text          NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    gender          text          NOT NULL DEFAULT '' CHECK (gender IN ('', 'M', 'F')),
    phone           text          NOT NULL DEFAULT '' CHECK (char_length(phone) <= 20),
    email           text          NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    address         text          NOT NULL DEFAULT '' CHECK (char_length(address) <= 300),
    district        text          NOT NULL DEFAULT '' CHECK (char_length(district) <= 100),
    city            text          NOT NULL DEFAULT '' CHECK (char_length(city) <= 100),
    province        text          NOT NULL DEFAULT '' CHECK (char_length(province) <= 100),
    postal_code     text          NOT NULL DEFAULT '' CHECK (char_length(postal_code) <= 10),
    credit_limit    numeric(18,2) NOT NULL DEFAULT 0 CHECK (credit_limit >= 0),  -- batas total piutang; 0 = tanpa batas (dipakai Fase 5.3)
    due_days        int           NOT NULL DEFAULT 0 CHECK (due_days BETWEEN 0 AND 3650), -- jatuh tempo piutang (hari); 0 = tanpa jatuh tempo
    valid_until     date,                                                        -- NULL = selalu aktif
    active          boolean       NOT NULL DEFAULT true,
    notes           text          NOT NULL DEFAULT '' CHECK (char_length(notes) <= 1000),
    cover_image_id  uuid,                                                        -- foto cover (berkas di Storage), NULL = belum ada
    points          int           NOT NULL DEFAULT 0 CHECK (points >= 0),
    lifetime_points int           NOT NULL DEFAULT 0 CHECK (lifetime_points >= 0),
    created_at      timestamptz   NOT NULL DEFAULT now(),
    updated_at      timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT members_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX members_code_key ON members (tenant_id, lower(code));
CREATE INDEX members_name_idx ON members (tenant_id, lower(name));
CREATE INDEX members_phone_idx ON members (tenant_id, phone) WHERE phone <> '';

CREATE TABLE member_point_movements (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id      uuid        NOT NULL,
    member_id      uuid        NOT NULL,
    kind           text        NOT NULL CHECK (kind IN ('EARN', 'REDEEM', 'ADJUST', 'REVERSAL')),
    points         int         NOT NULL CHECK (points <> 0),          -- selisih saldo (+/−)
    lifetime_delta int         NOT NULL DEFAULT 0,                    -- selisih lifetime_points (EARN +, pembalikannya −)
    balance_after  int         NOT NULL CHECK (balance_after >= 0),
    ref_type       text        NOT NULL CHECK (ref_type IN ('SALE', 'MANUAL')),
    ref_id         uuid,
    note           text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 200),
    actor_id       uuid,
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT member_point_movements_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX member_point_movements_member_idx ON member_point_movements (tenant_id, member_id, id DESC);
CREATE INDEX member_point_movements_ref_idx ON member_point_movements (tenant_id, ref_id) WHERE ref_id IS NOT NULL;

ALTER TABLE member_levels ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON member_levels USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
ALTER TABLE member_counters ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON member_counters USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
ALTER TABLE members ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON members USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Ledger poin append-only: hanya SELECT + INSERT; koreksi = movement baru.
ALTER TABLE member_point_movements ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_read   ON member_point_movements FOR SELECT USING (tenant_id = app_tenant_id());
CREATE POLICY tenant_insert ON member_point_movements FOR INSERT WITH CHECK (tenant_id = app_tenant_id());
REVOKE UPDATE, DELETE, TRUNCATE ON member_point_movements FROM aciraba_app;

CREATE TRIGGER member_levels_set_updated_at BEFORE UPDATE ON member_levels FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER members_set_updated_at BEFORE UPDATE ON members FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Level bawaan untuk tenant yang sudah ada (tenant baru diisi oleh Register). Aturan awal; pemilik bebas mengubahnya.
INSERT INTO member_levels (tenant_id, name, min_points, spend_per_point, point_value)
SELECT t.id, v.name, v.min_points, v.spend, 100
FROM tenants t
CROSS JOIN (VALUES ('Reguler', 0, 10000), ('Silver', 100, 8000), ('Gold', 500, 6000), ('Platinum', 1000, 5000)) AS v (name, min_points, spend);

-- Penjualan: member dan poin pada nota. `discount` sudah memuat potongan hasil tukar poin (`redeem_amount`).
ALTER TABLE sales ADD COLUMN member_id uuid;
ALTER TABLE sales ADD COLUMN points_earned   int NOT NULL DEFAULT 0 CHECK (points_earned >= 0);
ALTER TABLE sales ADD COLUMN points_redeemed int NOT NULL DEFAULT 0 CHECK (points_redeemed >= 0);
ALTER TABLE sales ADD COLUMN redeem_amount   numeric(18,2) NOT NULL DEFAULT 0 CHECK (redeem_amount >= 0);
ALTER TABLE sales ADD CONSTRAINT sales_member_fk FOREIGN KEY (tenant_id, member_id) REFERENCES members (tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE sales ADD CONSTRAINT sales_member_points_chk CHECK (member_id IS NOT NULL OR (points_earned = 0 AND points_redeemed = 0));
CREATE INDEX sales_member_idx ON sales (tenant_id, member_id, created_at DESC) WHERE member_id IS NOT NULL;

-- +goose Down
ALTER TABLE sales DROP CONSTRAINT sales_member_points_chk, DROP CONSTRAINT sales_member_fk;
ALTER TABLE sales DROP COLUMN redeem_amount, DROP COLUMN points_redeemed, DROP COLUMN points_earned, DROP COLUMN member_id;
DROP TABLE member_point_movements;
DROP TABLE members;
DROP TABLE member_counters;
DROP TABLE member_levels;
