-- +goose Up

-- Pustaka template landing page milik PLATFORM: Platform Admin menulis HTML + CSS template, tenant memilih lalu memakainya
-- sebagai titik awal editor (isinya disalin ke halaman tenant, bukan ditautkan). Tabel dibaca aplikasi, ditulis hanya lewat
-- fungsi di bawah (Platform Admin) atau storefront_template_seed (template bawaan, idempoten).
CREATE TABLE storefront_templates (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code        text        NOT NULL,
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 300),
    category    text        NOT NULL DEFAULT '' CHECK (char_length(category) <= 40),
    html        text        NOT NULL CHECK (char_length(html) <= 60000),
    css         text        NOT NULL DEFAULT '' CHECK (char_length(css) <= 20000),
    active      boolean     NOT NULL DEFAULT true,
    sort_order  int         NOT NULL DEFAULT 0,
    updated_by  uuid,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT storefront_templates_code_key UNIQUE (code),
    CONSTRAINT storefront_templates_code_format CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,39}$')
);
CREATE TRIGGER storefront_templates_set_updated_at BEFORE UPDATE ON storefront_templates FOR EACH ROW EXECUTE FUNCTION set_updated_at();
REVOKE ALL ON storefront_templates FROM aciraba_app;
GRANT SELECT ON storefront_templates TO aciraba_app;

-- +goose StatementBegin
-- Simpan template (p_id NULL = baru; code tidak berubah saat ubah). Pelaku harus Platform Admin aktif.
CREATE FUNCTION platform_storefront_template_save(
    p_actor uuid, p_id uuid, p_code text, p_name text, p_description text, p_category text, p_html text, p_css text,
    p_active boolean, p_sort int
) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    IF p_id IS NULL THEN
        INSERT INTO storefront_templates (code, name, description, category, html, css, active, sort_order, updated_by)
        VALUES (p_code, p_name, p_description, p_category, p_html, p_css, p_active, p_sort, p_actor)
        RETURNING id INTO v_id;
    ELSE
        UPDATE storefront_templates SET name = p_name, description = p_description, category = p_category, html = p_html,
               css = p_css, active = p_active, sort_order = p_sort, updated_by = p_actor
         WHERE id = p_id RETURNING id INTO v_id;
    END IF;
    RETURN v_id;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION platform_storefront_template_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM storefront_templates WHERE id = p_id;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- Template bawaan dari kode: hanya menambah bila kode belum ada (tidak menimpa hasil suntingan Platform Admin).
CREATE FUNCTION storefront_template_seed(p_code text, p_name text, p_description text, p_category text, p_html text, p_css text, p_sort int)
RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    INSERT INTO storefront_templates (code, name, description, category, html, css, sort_order)
    VALUES (p_code, p_name, p_description, p_category, p_html, p_css, p_sort)
    ON CONFLICT (code) DO NOTHING;
    RETURN FOUND;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform_storefront_template_save(uuid, uuid, text, text, text, text, text, text, boolean, int),
    platform_storefront_template_delete(uuid, uuid), storefront_template_seed(text, text, text, text, text, text, int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform_storefront_template_save(uuid, uuid, text, text, text, text, text, text, boolean, int),
    platform_storefront_template_delete(uuid, uuid), storefront_template_seed(text, text, text, text, text, text, int) TO aciraba_app;

-- +goose Down
DROP FUNCTION storefront_template_seed(text, text, text, text, text, text, int);
DROP FUNCTION platform_storefront_template_delete(uuid, uuid);
DROP FUNCTION platform_storefront_template_save(uuid, uuid, text, text, text, text, text, text, boolean, int);
DROP TABLE storefront_templates;
