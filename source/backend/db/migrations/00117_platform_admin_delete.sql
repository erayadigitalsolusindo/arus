-- +goose Up
-- +goose StatementBegin
-- Hapus permanen satu Platform Admin. Ditolak bila diri sendiri atau admin aktif terakhir. Admin yang pernah membuat/mengisi
-- checklist QA tertahan FK ON DELETE RESTRICT (SQLSTATE 23503): pakai nonaktifkan. Jejak audit platform tidak ber-FK dan
-- menyimpan admin_name, jadi riwayat tetap utuh; kode pemulihan 2FA ikut terhapus (CASCADE).
CREATE FUNCTION platform_admin_delete(p_actor uuid, p_target uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    PERFORM pg_advisory_xact_lock(80080001);
    IF p_actor = p_target THEN
        RAISE EXCEPTION 'tidak boleh menghapus diri sendiri' USING ERRCODE = 'P0001';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM platform_admins WHERE active AND id <> p_target) THEN
        RAISE EXCEPTION 'admin aktif terakhir' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM platform_admins WHERE id = p_target;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_admin_delete(uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_admin_delete(uuid, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_admin_delete(uuid, uuid);
