-- +goose Up

-- Poin keunggulan bebas per paket (teks yang ditulis Platform Admin, satu poin per elemen), tampil di kartu paket halaman
-- Langganan di samping daftar fitur otomatis. Panjang tiap poin (<= 120 karakter) dan jumlahnya divalidasi di aplikasi.
ALTER TABLE plans ADD COLUMN highlights text[] NOT NULL DEFAULT '{}';
ALTER TABLE plans ADD CONSTRAINT plans_highlights_chk CHECK (cardinality(highlights) <= 12);

DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[]);

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

-- Isi awal untuk dua paket bawaan (hanya bila belum ada poin).
UPDATE plans SET highlights = ARRAY['Pembayaran QRIS EraPay untuk toko Anda', 'Cocok untuk satu toko yang baru berjualan']
 WHERE code = 'pro' AND cardinality(highlights) = 0;
UPDATE plans SET highlights = ARRAY['Transaksi dan pengguna tanpa batas', 'Laporan keuangan dan akuntansi lengkap', 'Asisten AI HalloArus untuk analisis usaha']
 WHERE code = 'full' AND cardinality(highlights) = 0;

-- +goose Down
DROP FUNCTION platform_save_plan(uuid, uuid, text, text, text, numeric, int, int, int, boolean, boolean, int, text[], text[]);

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
ALTER TABLE plans DROP CONSTRAINT plans_highlights_chk;
ALTER TABLE plans DROP COLUMN highlights;
