-- +goose Up
-- +goose StatementBegin

-- Hapus tenant permanen oleh Platform Admin. Alurnya dua tahap: (1) jadwalkan hapus (tenant wajib nonaktif; masa tunggu
-- p_days ditentukan admin, 0 = langsung), (2) purge setelah jatuh tempo (manual oleh admin atau otomatis oleh penjadwal API).
-- Selama masa tunggu admin bisa membatalkan; mengaktifkan tenant kembali juga membatalkan jadwal.
ALTER TABLE tenants
    ADD COLUMN delete_at           timestamptz,
    ADD COLUMN delete_requested_at timestamptz,
    ADD CONSTRAINT tenants_delete_pair CHECK ((delete_at IS NULL) = (delete_requested_at IS NULL));

CREATE FUNCTION platform_tenant_name(p_tenant uuid) RETURNS TABLE (name text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$ SELECT t.name FROM tenants t WHERE t.id = p_tenant $$;

CREATE FUNCTION platform_tenant_delete_info(p_actor uuid, p_tenant uuid) RETURNS TABLE (delete_at timestamptz, delete_requested_at timestamptz)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY SELECT t.delete_at, t.delete_requested_at FROM tenants t WHERE t.id = p_tenant;
END
$$;

-- Jadwalkan hapus. Tenant aktif ditolak (P0001 'tenant masih aktif'); tenant tak ada -> baris kosong (NULL).
CREATE FUNCTION platform_tenant_schedule_delete(p_actor uuid, p_tenant uuid, p_days integer) RETURNS timestamptz
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_active boolean;
    v_at     timestamptz;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_days < 0 OR p_days > 365 THEN
        RAISE EXCEPTION 'masa tunggu tidak valid' USING ERRCODE = '22023';
    END IF;
    SELECT active INTO v_active FROM tenants WHERE id = p_tenant FOR UPDATE;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    IF v_active THEN
        RAISE EXCEPTION 'tenant masih aktif' USING ERRCODE = 'P0001';
    END IF;
    v_at := now() + make_interval(days => p_days);
    UPDATE tenants SET delete_at = v_at, delete_requested_at = COALESCE(delete_requested_at, now()) WHERE id = p_tenant;
    RETURN v_at;
END
$$;

CREATE FUNCTION platform_tenant_cancel_delete(p_actor uuid, p_tenant uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE tenants SET delete_at = NULL, delete_requested_at = NULL WHERE id = p_tenant AND delete_at IS NOT NULL;
    RETURN FOUND;
END
$$;

-- Inti: hapus SEMUA baris tenant (tabel ber-kolom tenant_id kecuali audit platform, yang sengaja bertahan) lalu barisnya
-- sendiri. Urutan dicari dari katalog FK (anak dulu), jadi tabel baru ikut terhapus tanpa mengubah fungsi ini.
-- Tanpa pemeriksaan admin: dipanggil oleh dua fungsi di bawah yang sudah memeriksa jadwal jatuh tempo.
CREATE FUNCTION tenant_purge_core(p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_tables  oid[];
    v_pending oid[];
    v_ready   oid[];
    v_oid     oid;
    v_name    text;
    v_has     boolean;
    v_fks     text[];
    v_fk      text;
    v_n       bigint;
    v_counts  jsonb := '{}';
    v_images  jsonb := '[]';
BEGIN
    SELECT array_agg(c.oid) INTO v_tables
      FROM pg_class c
     WHERE c.relkind = 'r' AND c.relnamespace = 'public'::regnamespace AND c.relname <> 'platform_audit_log'
       AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped);

    -- Berkas gambar barang ada di penyimpanan, bukan di DB: kembalikan id-nya agar aplikasi menghapusnya setelah commit.
    SELECT COALESCE(jsonb_agg(id), '[]') INTO v_images FROM item_images WHERE tenant_id = p_tenant;

    v_pending := v_tables;
    WHILE COALESCE(array_length(v_pending, 1), 0) > 0 LOOP
        -- Siap dihapus = tidak ada tabel tertunda lain (selain dirinya) yang merujuknya lewat FK.
        SELECT array_agg(t) INTO v_ready
          FROM unnest(v_pending) AS t
         WHERE NOT EXISTS (SELECT 1 FROM pg_constraint k
                            WHERE k.contype = 'f' AND k.confrelid = t AND k.conrelid <> t AND k.conrelid = ANY (v_pending));
        IF v_ready IS NULL THEN
            RAISE EXCEPTION 'siklus FK tak terselesaikan pada hapus tenant' USING ERRCODE = 'P0001';
        END IF;
        FOREACH v_oid IN ARRAY v_ready LOOP
            v_name := v_oid::regclass::text;
            -- FK self-referensi (nota revisi saling menunjuk, akun induk, jurnal balik) membentuk siklus yang tak bisa
            -- diurai per baris (RESTRICT) dan tak bisa dikosongkan (CHECK status). Lepas sementara, hapus, pasang ulang
            -- NOT VALID lalu VALIDATE (DDL transaksional; hanya untuk tabel yang memang punya baris tenant ini).
            v_fks := NULL;
            EXECUTE format('SELECT EXISTS (SELECT 1 FROM %s WHERE tenant_id = $1)', v_name) INTO v_has USING p_tenant;
            IF v_has THEN
                SELECT array_agg(k.conname || E'	' || pg_get_constraintdef(k.oid)) INTO v_fks
                  FROM pg_constraint k WHERE k.contype = 'f' AND k.conrelid = v_oid AND k.confrelid = v_oid;
                FOREACH v_fk IN ARRAY COALESCE(v_fks, '{}') LOOP
                    EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', v_name, split_part(v_fk, E'	', 1));
                END LOOP;
            END IF;
            EXECUTE format('DELETE FROM %s WHERE tenant_id = $1', v_name) USING p_tenant;
            GET DIAGNOSTICS v_n = ROW_COUNT;
            FOREACH v_fk IN ARRAY COALESCE(v_fks, '{}') LOOP
                EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s NOT VALID', v_name, split_part(v_fk, E'	', 1), split_part(v_fk, E'	', 2));
                EXECUTE format('ALTER TABLE %s VALIDATE CONSTRAINT %I', v_name, split_part(v_fk, E'	', 1));
            END LOOP;
            IF v_n > 0 THEN
                v_counts := v_counts || jsonb_build_object(v_name, v_n);
            END IF;
        END LOOP;
        v_pending := ARRAY(SELECT x FROM unnest(v_pending) AS x WHERE x <> ALL (v_ready));
    END LOOP;

    DELETE FROM tenants WHERE id = p_tenant;
    RETURN jsonb_build_object('rows', v_counts, 'item_images', v_images);
END
$$;

-- Purge manual oleh admin: hanya bila sudah dijadwalkan dan jatuh tempo, tenant nonaktif. NULL = belum boleh / tak ada.
CREATE FUNCTION platform_tenant_purge(p_actor uuid, p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_ok boolean;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    SELECT NOT active AND delete_at IS NOT NULL AND delete_at <= now() INTO v_ok FROM tenants WHERE id = p_tenant FOR UPDATE;
    IF v_ok IS NOT TRUE THEN
        RETURN NULL;
    END IF;
    RETURN tenant_purge_core(p_tenant);
END
$$;

-- Penjadwal API: tenant nonaktif yang sudah jatuh tempo (paling lama dulu).
CREATE FUNCTION platform_tenant_due_purges(p_limit integer) RETURNS TABLE (id uuid, name text)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
    SELECT t.id, t.name FROM tenants t
     WHERE NOT t.active AND t.delete_at IS NOT NULL AND t.delete_at <= now()
     ORDER BY t.delete_at LIMIT p_limit
$$;

CREATE FUNCTION platform_tenant_purge_due(p_tenant uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_ok boolean;
BEGIN
    SELECT NOT active AND delete_at IS NOT NULL AND delete_at <= now() INTO v_ok FROM tenants WHERE id = p_tenant FOR UPDATE;
    IF v_ok IS NOT TRUE THEN
        RETURN NULL;
    END IF;
    RETURN tenant_purge_core(p_tenant);
END
$$;

REVOKE ALL ON FUNCTION
    platform_tenant_name(uuid),
    platform_tenant_delete_info(uuid, uuid),
    platform_tenant_schedule_delete(uuid, uuid, integer),
    platform_tenant_cancel_delete(uuid, uuid),
    tenant_purge_core(uuid),
    platform_tenant_purge(uuid, uuid),
    platform_tenant_due_purges(integer),
    platform_tenant_purge_due(uuid)
FROM PUBLIC;
-- tenant_purge_core sengaja tanpa GRANT (hanya dipanggil fungsi lain milik pemilik skema).
GRANT EXECUTE ON FUNCTION
    platform_tenant_name(uuid),
    platform_tenant_delete_info(uuid, uuid),
    platform_tenant_schedule_delete(uuid, uuid, integer),
    platform_tenant_cancel_delete(uuid, uuid),
    platform_tenant_purge(uuid, uuid),
    platform_tenant_due_purges(integer),
    platform_tenant_purge_due(uuid)
TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION platform_tenant_purge_due(uuid);
DROP FUNCTION platform_tenant_due_purges(integer);
DROP FUNCTION platform_tenant_purge(uuid, uuid);
DROP FUNCTION tenant_purge_core(uuid);
DROP FUNCTION platform_tenant_cancel_delete(uuid, uuid);
DROP FUNCTION platform_tenant_schedule_delete(uuid, uuid, integer);
DROP FUNCTION platform_tenant_delete_info(uuid, uuid);
DROP FUNCTION platform_tenant_name(uuid);
ALTER TABLE tenants DROP CONSTRAINT tenants_delete_pair, DROP COLUMN delete_requested_at, DROP COLUMN delete_at;
-- +goose StatementEnd
