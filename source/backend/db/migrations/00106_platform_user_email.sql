-- +goose Up

-- Platform Admin mengganti email login pengguna tenant. Tautan login Google lama harus lepas: identitas itu milik pemilik
-- email lama, dan login Google menautkan akun lewat email, jadi tanpa ini pemilik lama tetap bisa masuk lewat Google.
-- user_identities tanpa hak bagi aciraba_app, jadi lewat fungsi SECURITY DEFINER yang memeriksa admin platform aktif.
-- +goose StatementBegin
CREATE FUNCTION platform_user_unlink_identities(p_actor uuid, p_tenant uuid, p_user uuid) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    n integer;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM user_identities WHERE tenant_id = p_tenant AND user_id = p_user;
    GET DIAGNOSTICS n = ROW_COUNT;
    RETURN n;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_user_unlink_identities(uuid, uuid, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_user_unlink_identities(uuid, uuid, uuid) TO aciraba_app;

-- +goose Down
DROP FUNCTION platform_user_unlink_identities(uuid, uuid, uuid);
