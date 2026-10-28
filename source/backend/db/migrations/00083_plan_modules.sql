-- +goose Up

-- Modul (id izin, authz.Modules) yang boleh dipakai tenant pada paket ini. NULL = semua modul. Daftar eksplisit berarti
-- modul di luar daftar terkunci untuk SEMUA pengguna tenant (termasuk Owner), apa pun role-nya; izin efektif = izin role ∩ modul paket.
-- Dibaca bersama paket efektif (satu baris per tenant, kunci utama) saat izin pengguna dimuat; tidak ada tabel tambahan.
ALTER TABLE plans ADD COLUMN modules text[];
ALTER TABLE plans ADD CONSTRAINT plans_modules_chk CHECK (modules IS NULL OR cardinality(modules) <= 200);

-- Tambah parameter p_modules: fungsi lama diganti (tanda tangan berubah).
DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int);

-- +goose StatementBegin
CREATE FUNCTION platform_save_plan(
    p_actor uuid, p_id uuid, p_code text, p_name text, p_description text, p_price numeric, p_duration int,
    p_max_tx int, p_max_users int, p_self_service boolean, p_active boolean, p_sort int, p_modules text[]
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO plans (code, name, description, price, duration_days, max_tx_per_day, max_users, self_service, active, sort_order, modules)
        VALUES (p_code, p_name, p_description, p_price, p_duration, p_max_tx, p_max_users, p_self_service, p_active, p_sort, p_modules)
        RETURNING id INTO v_id;
    ELSE
        UPDATE plans SET name = p_name, description = p_description, price = p_price, duration_days = p_duration,
               max_tx_per_day = p_max_tx, max_users = p_max_users, self_service = p_self_service,
               active = (is_default OR p_active), sort_order = p_sort, modules = p_modules
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[]) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[]);

-- +goose StatementBegin
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
        UPDATE plans SET name = p_name, description = p_description, price = p_price, duration_days = p_duration,
               max_tx_per_day = p_max_tx, max_users = p_max_users, self_service = p_self_service,
               active = (is_default OR p_active), sort_order = p_sort
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int) TO aciraba_app;
ALTER TABLE plans DROP CONSTRAINT plans_modules_chk;
ALTER TABLE plans DROP COLUMN modules;
