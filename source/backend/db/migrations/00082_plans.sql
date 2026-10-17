-- +goose Up

-- Paket langganan tenant (model bisnis: FREE terbatas, paket berbayar dibayar via QRIS EraPay). Paket DINAMIS:
-- operator platform menambah/mengubah paket sendiri. Batas NULL = tanpa batas (paket enterprise / on-premise).
CREATE TABLE plans (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code           text          NOT NULL,
    name           text          NOT NULL,
    description    text          NOT NULL DEFAULT '',
    price          numeric(18,2) NOT NULL DEFAULT 0 CHECK (price >= 0),
    -- Masa aktif per pembelian (hari). NULL = tanpa kedaluwarsa (FREE / enterprise yang dipasang manual).
    duration_days  int           CHECK (duration_days IS NULL OR duration_days BETWEEN 1 AND 3650),
    max_tx_per_day int           CHECK (max_tx_per_day IS NULL OR max_tx_per_day > 0),
    max_users      int           CHECK (max_users IS NULL OR max_users > 0),
    -- Paket yang dipakai tenant tanpa langganan aktif / yang sudah kedaluwarsa (tepat satu).
    is_default     boolean       NOT NULL DEFAULT false,
    -- true = tampil di halaman upgrade tenant dan bisa dibeli sendiri via QRIS (harga > 0 dan duration_days wajib).
    self_service   boolean       NOT NULL DEFAULT false,
    active         boolean       NOT NULL DEFAULT true,
    sort_order     int           NOT NULL DEFAULT 0,
    created_at     timestamptz   NOT NULL DEFAULT now(),
    updated_at     timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT plans_code_key    UNIQUE (code),
    CONSTRAINT plans_code_format CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,31}$'),
    CONSTRAINT plans_name_len    CHECK (char_length(name) BETWEEN 1 AND 60),
    CONSTRAINT plans_desc_len    CHECK (char_length(description) <= 300),
    CONSTRAINT plans_self_service_chk CHECK (NOT self_service OR (price > 0 AND duration_days IS NOT NULL AND active)),
    CONSTRAINT plans_default_chk CHECK (NOT is_default OR (active AND NOT self_service))
);
CREATE UNIQUE INDEX plans_one_default ON plans (is_default) WHERE is_default;
CREATE TRIGGER plans_set_updated_at BEFORE UPDATE ON plans FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, is_default, self_service, sort_order) VALUES
    ('free', 'FREE', 'Untuk mencoba: 100 transaksi per hari, 2 pengguna.', 0, NULL, 100, 2, true, false, 1),
    ('pro', 'PRO', '300 transaksi per hari, 4 pengguna.', 20000, 30, 300, 4, false, true, 2);

-- Plans dibaca semua tenant (halaman upgrade); penulisan hanya lewat fungsi Platform Admin di bawah.
REVOKE ALL ON plans FROM aciraba_app;
GRANT SELECT ON plans TO aciraba_app;

-- Langganan tenant. Tanpa baris / kedaluwarsa = paket default (FREE). Override = batas khusus tenant (NULL = ikut paket).
CREATE TABLE tenant_subscriptions (
    tenant_id                uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE RESTRICT,
    plan_id                  uuid        NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    expires_at               timestamptz,
    override_max_tx_per_day  int         CHECK (override_max_tx_per_day IS NULL OR override_max_tx_per_day > 0),
    override_max_users       int         CHECK (override_max_users IS NULL OR override_max_users > 0),
    note                     text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 300),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tenant_subscriptions_plan_idx ON tenant_subscriptions (plan_id);
CREATE TRIGGER tenant_subscriptions_set_updated_at BEFORE UPDATE ON tenant_subscriptions FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE tenant_subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_subscriptions USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
REVOKE ALL ON tenant_subscriptions FROM aciraba_app;
GRANT SELECT ON tenant_subscriptions TO aciraba_app;

-- Pembayaran paket via QRIS EraPay platform. Aplikasi menulis; aktivasi langganan hanya lewat subscription_activate().
CREATE TABLE subscription_payments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid          NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    plan_id           uuid          NOT NULL REFERENCES plans (id) ON DELETE RESTRICT,
    idempotency_key   text          NOT NULL,
    amount            numeric(18,2) NOT NULL CHECK (amount > 0),
    duration_days     int           NOT NULL CHECK (duration_days BETWEEN 1 AND 3650),
    status            text          NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','succeeded','failed','expired','cancelled','unknown')),
    erapay_payment_id text,
    qr_value          text,
    expires_at        timestamptz,
    created_by        uuid,
    applied_at        timestamptz,
    created_at        timestamptz   NOT NULL DEFAULT now(),
    updated_at        timestamptz   NOT NULL DEFAULT now(),
    CONSTRAINT subscription_payments_idem_key UNIQUE (tenant_id, idempotency_key)
);
CREATE INDEX subscription_payments_tenant_idx ON subscription_payments (tenant_id, created_at DESC);
-- Rekonsiliasi: pembayaran yang belum selesai (dibaca lintas tenant lewat fungsi di bawah).
CREATE INDEX subscription_payments_open_idx ON subscription_payments (created_at) WHERE status IN ('pending','unknown') AND erapay_payment_id IS NOT NULL;
CREATE INDEX subscription_payments_unapplied_idx ON subscription_payments (created_at) WHERE status = 'succeeded' AND applied_at IS NULL;
CREATE TRIGGER subscription_payments_set_updated_at BEFORE UPDATE ON subscription_payments FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE subscription_payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscription_payments USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- Penghitung transaksi per hari (hari = tanggal lokal outlet tempat nota dibuat). Naik atomik di transaksi nota.
CREATE TABLE tenant_daily_tx (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    day       date NOT NULL,
    tx_count  int  NOT NULL DEFAULT 0 CHECK (tx_count >= 0),
    PRIMARY KEY (tenant_id, day)
);
ALTER TABLE tenant_daily_tx ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_daily_tx USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- ---- fungsi ----

-- +goose StatementBegin
-- Simpan paket (p_id NULL = baru). Pelaku harus Platform Admin aktif. Paket default tidak boleh dinonaktifkan / dijual sendiri.
CREATE FUNCTION platform_save_plan(
    p_actor uuid, p_id uuid, p_code text, p_name text, p_description text, p_price numeric, p_duration int,
    p_max_tx int, p_max_users int, p_self_service boolean, p_active boolean, p_sort int
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, self_service, active, sort_order)
        VALUES (p_code, p_name, p_description, p_price, p_duration, p_max_tx, p_max_users, p_self_service, p_active, p_sort)
        RETURNING id INTO v_id;
    ELSE
        -- code tidak berubah (kunci stabil untuk integrasi); paket default selalu aktif.
        UPDATE plans SET name = p_name, description = p_description, price = p_price, duration_days = p_duration,
               max_tx_per_day = p_max_tx, max_users = p_max_users, self_service = p_self_service,
               active = (is_default OR p_active), sort_order = p_sort
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Pasang paket ke tenant secara manual (gratis/trial/enterprise). p_plan NULL = kembali ke paket default (hapus langganan).
CREATE FUNCTION platform_set_tenant_plan(
    p_actor uuid, p_tenant uuid, p_plan uuid, p_expires timestamptz, p_ov_tx int, p_ov_users int, p_note text
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_plan IS NULL THEN
        DELETE FROM tenant_subscriptions WHERE tenant_id = p_tenant;
        RETURN true;
    END IF;
    INSERT INTO tenant_subscriptions (tenant_id, plan_id, expires_at, override_max_tx_per_day, override_max_users, note)
    VALUES (p_tenant, p_plan, p_expires, p_ov_tx, p_ov_users, p_note)
    ON CONFLICT (tenant_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, expires_at = EXCLUDED.expires_at,
        override_max_tx_per_day = EXCLUDED.override_max_tx_per_day, override_max_users = EXCLUDED.override_max_users,
        note = EXCLUDED.note;
    RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Aktifkan langganan dari satu pembayaran yang LUNAS (sekali saja). Memperpanjang dari tanggal berakhir bila paket sama dan
-- masih aktif, selain itu dihitung dari sekarang. Override batas khusus dibuang (paket baru = batas paket). Mengembalikan
-- tanggal berakhir baru; NULL bila pembayaran tidak ada / belum lunas / sudah pernah diterapkan.
CREATE FUNCTION subscription_activate(p_tenant uuid, p_payment uuid) RETURNS timestamptz
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    pay subscription_payments%ROWTYPE;
    cur tenant_subscriptions%ROWTYPE;
    base timestamptz := now();
    v_exp timestamptz;
BEGIN
    SELECT * INTO pay FROM subscription_payments WHERE id = p_payment AND tenant_id = p_tenant FOR UPDATE;
    IF NOT FOUND OR pay.status <> 'succeeded' OR pay.applied_at IS NOT NULL THEN
        RETURN NULL;
    END IF;
    SELECT * INTO cur FROM tenant_subscriptions WHERE tenant_id = p_tenant FOR UPDATE;
    IF FOUND AND cur.plan_id = pay.plan_id AND cur.expires_at IS NOT NULL AND cur.expires_at > base THEN
        base := cur.expires_at;
    END IF;
    v_exp := base + make_interval(days => pay.duration_days);
    INSERT INTO tenant_subscriptions (tenant_id, plan_id, expires_at)
    VALUES (p_tenant, pay.plan_id, v_exp)
    ON CONFLICT (tenant_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, expires_at = EXCLUDED.expires_at,
        override_max_tx_per_day = NULL, override_max_users = NULL, note = '';
    UPDATE subscription_payments SET applied_at = now() WHERE id = p_payment;
    RETURN v_exp;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Daftar pembayaran yang perlu direkonsiliasi lintas tenant (penjadwal): belum final di EraPay, atau lunas tapi belum diterapkan.
CREATE FUNCTION subscription_reconcile_targets(p_limit int) RETURNS TABLE (tenant_id uuid, id uuid, erapay_payment_id text, status text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT sp.tenant_id, sp.id, sp.erapay_payment_id, sp.status
      FROM subscription_payments sp
     WHERE (sp.status IN ('pending','unknown') AND sp.erapay_payment_id IS NOT NULL AND sp.created_at > now() - interval '3 days')
        OR (sp.status = 'succeeded' AND sp.applied_at IS NULL)
     ORDER BY sp.created_at
     LIMIT p_limit
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION
    platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int),
    platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text),
    subscription_activate(uuid, uuid),
    subscription_reconcile_targets(int)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int),
    platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text),
    subscription_activate(uuid, uuid),
    subscription_reconcile_targets(int)
TO aciraba_app;

-- +goose Down
DROP FUNCTION subscription_reconcile_targets(int);
DROP FUNCTION subscription_activate(uuid, uuid);
DROP FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text);
DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int);
DROP TABLE tenant_daily_tx;
DROP TABLE subscription_payments;
DROP TABLE tenant_subscriptions;
DROP TABLE plans;
