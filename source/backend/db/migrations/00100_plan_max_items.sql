-- +goose Up

-- Batas jumlah barang AKTIF per paket (selain transaksi/hari dan pengguna). NULL = tanpa batas; paket yang sudah ada tidak
-- berubah (Platform Admin mengisinya sendiri). Barang yang diarsipkan tidak dihitung. Override per tenant NULL = ikut paket.
ALTER TABLE plans ADD COLUMN max_items int CHECK (max_items IS NULL OR max_items > 0);
ALTER TABLE tenant_subscriptions ADD COLUMN override_max_items int CHECK (override_max_items IS NULL OR override_max_items > 0);

-- Pemeriksaan batas menghitung barang aktif dengan LIMIT (paling banyak batas+0 baris dibaca), jadi murah walau tenant punya
-- ratusan ribu barang; indeks parsial ini membuatnya index-only.
CREATE INDEX items_tenant_active_idx ON items (tenant_id) WHERE active;

DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[]);

-- +goose StatementBegin
CREATE FUNCTION platform_save_plan(
    p_actor uuid, p_id uuid, p_code text, p_name text, p_description text, p_price numeric, p_duration int,
    p_max_tx int, p_max_users int, p_self_service boolean, p_active boolean, p_sort int, p_modules text[], p_highlights text[],
    p_max_items int
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, self_service, active, sort_order, modules, highlights, max_items)
        VALUES (p_code, p_name, p_description, p_price, p_duration, p_max_tx, p_max_users, p_self_service, p_active, p_sort, p_modules, coalesce(p_highlights, '{}'), p_max_items)
        RETURNING id INTO v_id;
    ELSE
        UPDATE plans SET name = p_name, description = p_description, price = p_price, duration_days = p_duration,
               max_tx_per_day = p_max_tx, max_users = p_max_users, self_service = p_self_service,
               active = (is_default OR p_active), sort_order = p_sort, modules = p_modules, highlights = coalesce(p_highlights, '{}'),
               max_items = p_max_items
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[], int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[], int) TO aciraba_app;

DROP FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text);

-- +goose StatementBegin
CREATE FUNCTION platform_set_tenant_plan(
    p_actor uuid, p_tenant uuid, p_plan uuid, p_expires timestamptz, p_ov_tx int, p_ov_users int, p_note text, p_ov_items int
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_plan IS NULL THEN
        DELETE FROM tenant_subscriptions WHERE tenant_id = p_tenant;
        RETURN true;
    END IF;
    INSERT INTO tenant_subscriptions (tenant_id, plan_id, expires_at, override_max_tx_per_day, override_max_users, note, override_max_items)
    VALUES (p_tenant, p_plan, p_expires, p_ov_tx, p_ov_users, p_note, p_ov_items)
    ON CONFLICT (tenant_id) DO UPDATE SET plan_id = EXCLUDED.plan_id, expires_at = EXCLUDED.expires_at,
        override_max_tx_per_day = EXCLUDED.override_max_tx_per_day, override_max_users = EXCLUDED.override_max_users,
        note = EXCLUDED.note, override_max_items = EXCLUDED.override_max_items;
    RETURN true;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text, int) TO aciraba_app;

-- +goose StatementBegin
-- Sama seperti versi 00082, ditambah override_max_items dibuang saat paket baru diaktifkan.
CREATE OR REPLACE FUNCTION subscription_activate(p_tenant uuid, p_payment uuid) RETURNS timestamptz
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
        override_max_tx_per_day = NULL, override_max_users = NULL, override_max_items = NULL, note = '';
    UPDATE subscription_payments SET applied_at = now() WHERE id = p_payment;
    RETURN v_exp;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text, int);
-- +goose StatementBegin
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
REVOKE ALL ON FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_set_tenant_plan(uuid, uuid, uuid, timestamptz, int, int, text) TO aciraba_app;

DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[], int);
-- +goose StatementBegin
CREATE FUNCTION platform_save_plan(
    p_actor uuid, p_id uuid, p_code text, p_name text, p_description text, p_price numeric, p_duration int,
    p_max_tx int, p_max_users int, p_self_service boolean, p_active boolean, p_sort int, p_modules text[], p_highlights text[]
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, self_service, active, sort_order, modules, highlights)
        VALUES (p_code, p_name, p_description, p_price, p_duration, p_max_tx, p_max_users, p_self_service, p_active, p_sort, p_modules, coalesce(p_highlights, '{}'))
        RETURNING id INTO v_id;
    ELSE
        UPDATE plans SET name = p_name, description = p_description, price = p_price, duration_days = p_duration,
               max_tx_per_day = p_max_tx, max_users = p_max_users, self_service = p_self_service,
               active = (is_default OR p_active), sort_order = p_sort, modules = p_modules, highlights = coalesce(p_highlights, '{}')
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[]) TO aciraba_app;

DROP INDEX items_tenant_active_idx;
ALTER TABLE tenant_subscriptions DROP COLUMN override_max_items;
ALTER TABLE plans DROP COLUMN max_items;
