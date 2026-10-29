-- +goose Up
-- +goose StatementBegin

-- Checklist QA/QC platform. Daftar item (katalog) hidup di kode Go; DB hanya menyimpan "run" (satu putaran uji, mis. per
-- rilis) dan hasil per item. Tabel milik platform: aplikasi tidak punya hak langsung, semua lewat fungsi di bawah.
CREATE TABLE platform_qa_runs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    created_by  uuid        NOT NULL REFERENCES platform_admins (id) ON DELETE RESTRICT,
    created_at  timestamptz NOT NULL DEFAULT now(),
    closed_at   timestamptz
);
CREATE INDEX platform_qa_runs_created_idx ON platform_qa_runs (created_at DESC, id);

CREATE TABLE platform_qa_results (
    run_id      uuid        NOT NULL REFERENCES platform_qa_runs (id) ON DELETE CASCADE,
    item_key    text        NOT NULL CHECK (char_length(item_key) BETWEEN 1 AND 80),
    -- Item tanpa baris = belum diuji (todo).
    status      text        NOT NULL CHECK (status IN ('pass', 'fail', 'skip')),
    note        text        NOT NULL DEFAULT '' CHECK (char_length(note) <= 1000),
    updated_by  uuid        NOT NULL REFERENCES platform_admins (id) ON DELETE RESTRICT,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, item_key)
);

REVOKE ALL ON platform_qa_runs, platform_qa_results FROM aciraba_app;

-- Daftar run (p_id NULL = semua), terbaru dulu, dengan hitungan hasil. Jumlah run kecil (satu per rilis): tanpa keyset.
CREATE FUNCTION platform_qa_run_list(p_actor uuid, p_id uuid) RETURNS TABLE (
    id uuid, name text, created_by_name text, created_at timestamptz, closed_at timestamptz,
    pass_count bigint, fail_count bigint, skip_count bigint
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT r.id, r.name, a.name, r.created_at, r.closed_at,
           count(*) FILTER (WHERE x.status = 'pass'),
           count(*) FILTER (WHERE x.status = 'fail'),
           count(*) FILTER (WHERE x.status = 'skip')
      FROM platform_qa_runs r
      JOIN platform_admins a ON a.id = r.created_by
      LEFT JOIN platform_qa_results x ON x.run_id = r.id
     WHERE p_id IS NULL OR r.id = p_id
     GROUP BY r.id, a.name
     ORDER BY r.created_at DESC, r.id;
END
$$;

CREATE FUNCTION platform_qa_run_create(p_actor uuid, p_name text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_id uuid;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    INSERT INTO platform_qa_runs (name, created_by) VALUES (p_name, p_actor) RETURNING id INTO v_id;
    RETURN v_id;
END
$$;

-- Tutup (true) atau buka kembali (false) run. Run tertutup tidak menerima perubahan hasil.
CREATE FUNCTION platform_qa_run_close(p_actor uuid, p_id uuid, p_closed boolean) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    UPDATE platform_qa_runs SET closed_at = CASE WHEN p_closed THEN COALESCE(closed_at, now()) ELSE NULL END WHERE id = p_id;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_qa_run_delete(p_actor uuid, p_id uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    DELETE FROM platform_qa_runs WHERE id = p_id;
    RETURN FOUND;
END
$$;

CREATE FUNCTION platform_qa_result_list(p_actor uuid, p_run uuid) RETURNS TABLE (
    item_key text, status text, note text, updated_by_name text, updated_at timestamptz
)
LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS
$$
BEGIN
    PERFORM platform_assert_admin(p_actor);
    RETURN QUERY
    SELECT x.item_key, x.status, x.note, a.name, x.updated_at
      FROM platform_qa_results x JOIN platform_admins a ON a.id = x.updated_by
     WHERE x.run_id = p_run;
END
$$;

-- Simpan hasil satu item. p_status 'todo' = hapus hasil (kembali belum diuji). Mengembalikan:
-- 'ok', 'not_found' (run tidak ada), atau 'closed' (run sudah ditutup).
CREATE FUNCTION platform_qa_result_set(p_actor uuid, p_run uuid, p_key text, p_status text, p_note text) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS
$$
DECLARE
    v_closed timestamptz;
BEGIN
    PERFORM platform_assert_admin(p_actor);
    SELECT closed_at INTO v_closed FROM platform_qa_runs WHERE id = p_run FOR SHARE;
    IF NOT FOUND THEN RETURN 'not_found'; END IF;
    IF v_closed IS NOT NULL THEN RETURN 'closed'; END IF;
    IF p_status = 'todo' THEN
        DELETE FROM platform_qa_results WHERE run_id = p_run AND item_key = p_key;
    ELSE
        INSERT INTO platform_qa_results (run_id, item_key, status, note, updated_by)
        VALUES (p_run, p_key, p_status, p_note, p_actor)
        ON CONFLICT (run_id, item_key) DO UPDATE
           SET status = EXCLUDED.status, note = EXCLUDED.note, updated_by = EXCLUDED.updated_by, updated_at = now();
    END IF;
    RETURN 'ok';
END
$$;

REVOKE ALL ON FUNCTION
    platform_qa_run_list(uuid, uuid),
    platform_qa_run_create(uuid, text),
    platform_qa_run_close(uuid, uuid, boolean),
    platform_qa_run_delete(uuid, uuid),
    platform_qa_result_list(uuid, uuid),
    platform_qa_result_set(uuid, uuid, text, text, text)
FROM PUBLIC;
GRANT EXECUTE ON FUNCTION
    platform_qa_run_list(uuid, uuid),
    platform_qa_run_create(uuid, text),
    platform_qa_run_close(uuid, uuid, boolean),
    platform_qa_run_delete(uuid, uuid),
    platform_qa_result_list(uuid, uuid),
    platform_qa_result_set(uuid, uuid, text, text, text)
TO aciraba_app;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION platform_qa_result_set(uuid, uuid, text, text, text);
DROP FUNCTION platform_qa_result_list(uuid, uuid);
DROP FUNCTION platform_qa_run_delete(uuid, uuid);
DROP FUNCTION platform_qa_run_close(uuid, uuid, boolean);
DROP FUNCTION platform_qa_run_create(uuid, text);
DROP FUNCTION platform_qa_run_list(uuid, uuid);
DROP TABLE platform_qa_results;
DROP TABLE platform_qa_runs;
-- +goose StatementEnd
