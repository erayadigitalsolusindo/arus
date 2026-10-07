-- +goose Up

-- Gambar item (Fase 3.2, irisan C). Berkas JPG hasil olahan server disimpan di storage (disk lokal) dengan kunci
-- yang diturunkan dari id: <tenant_id>/<id>.jpg (penuh) dan <tenant_id>/<id>-t.jpg (thumbnail); tabel ini hanya
-- metadata. Satu gambar utama per item dijaga DB (indeks unik parsial). Batas jumlah gambar dijaga service dengan
-- mengunci baris item.

CREATE TABLE item_images (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL,
    item_id     uuid        NOT NULL,
    position    integer     NOT NULL CHECK (position >= 1),
    is_main     boolean     NOT NULL DEFAULT false,
    width       integer     NOT NULL CHECK (width > 0),
    height      integer     NOT NULL CHECK (height > 0),
    full_bytes  integer     NOT NULL CHECK (full_bytes > 0),
    thumb_bytes integer     NOT NULL CHECK (thumb_bytes > 0),
    created_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT item_images_tenant_id_id_key UNIQUE (tenant_id, id),
    CONSTRAINT item_images_item_fk FOREIGN KEY (tenant_id, item_id) REFERENCES items (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX item_images_one_main_key ON item_images (item_id) WHERE is_main;
CREATE INDEX item_images_item_idx ON item_images (tenant_id, item_id, position);

ALTER TABLE item_images ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON item_images USING (tenant_id = app_tenant_id()) WITH CHECK (tenant_id = app_tenant_id());

-- +goose Down
DROP TABLE item_images;
