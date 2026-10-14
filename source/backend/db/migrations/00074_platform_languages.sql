-- +goose Up
-- +goose StatementBegin

-- Bahasa yang didukung platform. ID dan EN tidak dapat dihapus (protected = true).
-- Bahasa baru ditambahkan lewat API Platform Admin, tapi terjemahan static file masih perlu dibuat manual.
CREATE TABLE platform_languages (
    code        text        PRIMARY KEY,
    name        text        NOT NULL,          -- contoh: 'Bahasa Indonesia'
    short       text        NOT NULL,          -- contoh: 'ID'
    intl        text        NOT NULL,          -- contoh: 'id-ID'
    protected   boolean     NOT NULL DEFAULT false,  -- true = ID & EN, tidak boleh dihapus
    active      boolean     NOT NULL DEFAULT true,
    sort_order  int         NOT NULL DEFAULT 0,
    created_at  timestamptz  NOT NULL DEFAULT now(),
    updated_at  timestamptz  NOT NULL DEFAULT now(),
    CONSTRAINT platform_languages_code_format CHECK (code ~ '^[a-z]{2}$'),
    CONSTRAINT platform_languages_short_len    CHECK (char_length(short) BETWEEN 2 AND 6),
    CONSTRAINT platform_languages_intl_format  CHECK (intl ~ '^[a-z]{2}-[A-Z]{2}$')
);

CREATE TRIGGER platform_languages_set_updated_at BEFORE UPDATE ON platform_languages
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Seed bahasa bawaan
INSERT INTO platform_languages (code, name, short, intl, protected, active, sort_order) VALUES
    ('id', 'Bahasa Indonesia', 'ID', 'id-ID', true,  true,  1),
    ('en', 'English',          'EN', 'en-US', true,  true,  2);

REVOKE ALL ON platform_languages FROM aciraba_app;
GRANT SELECT ON platform_languages TO aciraba_app;

-- Fungsi: hanya non-protected yang boleh dihapus
CREATE FUNCTION platform_delete_language(p_actor uuid, p_code text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF EXISTS (SELECT 1 FROM platform_languages WHERE code = p_code AND protected) THEN
        RAISE EXCEPTION 'bahasa protegido tidak dapat dihapus' USING ERRCODE = 'P0001';
    END IF;
    DELETE FROM platform_languages WHERE code = p_code;
    RETURN FOUND;
END
$$;

-- Fungsi: update bahasa (nama, active, sort_order). Code & protected tidak bisa diubah.
CREATE FUNCTION platform_update_language(p_actor uuid, p_code text, p_name text, p_active boolean, p_sort_order int) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE platform_languages
       SET name = p_name, active = p_active, sort_order = p_sort_order
     WHERE code = p_code;
    RETURN FOUND;
END
$$;

-- Fungsi: tambah bahasa baru (protected selalu false)
CREATE FUNCTION platform_create_language(p_actor uuid, p_code text, p_name text, p_short text, p_intl text, p_sort_order int) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_code text;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO platform_languages (code, name, short, intl, sort_order)
    VALUES (p_code, p_name, p_short, p_intl, p_sort_order)
    RETURNING code INTO v_code;
    RETURN v_code;
END
$$;

-- Fungsi: daftar bahasa (untuk API)
CREATE FUNCTION platform_list_languages(p_actor uuid) RETURNS TABLE (
    code text, name text, short text, intl text, protected boolean, active boolean, sort_order int, created_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT l.code, l.name, l.short, l.intl, l.protected, l.active, l.sort_order, l.created_at
    FROM platform_languages l ORDER BY l.sort_order, l.code;
END
$$;

REVOKE ALL ON FUNCTION platform_delete_language(uuid, text),
    platform_update_language(uuid, text, text, boolean, int),
    platform_create_language(uuid, text, text, text, text, int),
    platform_list_languages(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_delete_language(uuid, text),
    platform_update_language(uuid, text, text, boolean, int),
    platform_create_language(uuid, text, text, text, text, int),
    platform_list_languages(uuid) TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION platform_list_languages(uuid);
DROP FUNCTION platform_create_language(uuid, text, text, text, text, int);
DROP FUNCTION platform_update_language(uuid, text, text, boolean, int);
DROP FUNCTION platform_delete_language(uuid, text);
DROP TRIGGER platform_languages_set_updated_at ON platform_languages;
DROP TABLE platform_languages;
-- +goose StatementEnd