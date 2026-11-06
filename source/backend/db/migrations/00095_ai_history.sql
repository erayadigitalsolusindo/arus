-- +goose Up

-- Riwayat percakapan HalloArus. PRIVASI: judul dan isi pesan (serta kartu usulan aksi) disimpan terenkripsi AES-GCM di aplikasi (kunci
-- terpisah dari API key, AAD = tenant:percakapan); hanya pemilik percakapan yang boleh membaca (dijaga query: tenant_id + user_id, bukan
-- hanya RLS). Foto nota dan hasil tool mentah TIDAK disimpan. Retensi 90 hari + maksimal 50 percakapan per pengguna ditegakkan aplikasi.
CREATE TABLE ai_conversations (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL,
    title_enc  text        NOT NULL,
    msg_count  integer     NOT NULL DEFAULT 0 CHECK (msg_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ai_conversations_user_fk FOREIGN KEY (tenant_id, user_id) REFERENCES users (tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT ai_conversations_tenant_id_id_key UNIQUE (tenant_id, id)
);
CREATE INDEX ai_conversations_user_idx ON ai_conversations (tenant_id, user_id, updated_at DESC, id DESC);

CREATE TABLE ai_messages (
    conversation_id uuid        NOT NULL,
    tenant_id       uuid        NOT NULL,
    seq             integer     NOT NULL CHECK (seq >= 1),
    role            text        NOT NULL CHECK (role IN ('user', 'assistant')),
    content_enc     text        NOT NULL,
    -- Kartu usulan aksi (JSON terenkripsi) dan nama tool yang dipakai (tanpa isi hasil).
    actions_enc     text,
    tools           text[]      NOT NULL DEFAULT '{}',
    had_images      boolean     NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, seq),
    CONSTRAINT ai_messages_conv_fk FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE CASCADE
);

-- Status kartu usulan (kunci primer = id usulan) agar kartu di riwayat menampilkan Disetujui/Dibatalkan setelah halaman dimuat ulang.
CREATE TABLE ai_actions (
    id              uuid        PRIMARY KEY,
    tenant_id       uuid        NOT NULL,
    conversation_id uuid        NOT NULL,
    status          text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done', 'cancelled')),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ai_actions_conv_fk FOREIGN KEY (tenant_id, conversation_id) REFERENCES ai_conversations (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX ai_actions_conv_idx ON ai_actions (tenant_id, conversation_id);

-- Preferensi tenant: saklar simpan riwayat (bawaan menyala; pemilik bisa mematikan).
CREATE TABLE ai_prefs (
    tenant_id    uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    save_history boolean     NOT NULL DEFAULT true,
    updated_by   uuid,
    updated_at   timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE ai_conversations ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_actions ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_prefs ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ai_conversations USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON ai_messages USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON ai_actions USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
CREATE POLICY tenant_isolation ON ai_prefs USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON ai_conversations, ai_messages, ai_actions, ai_prefs TO aciraba_app;

-- +goose Down
DROP TABLE ai_prefs;
DROP TABLE ai_actions;
DROP TABLE ai_messages;
DROP TABLE ai_conversations;
