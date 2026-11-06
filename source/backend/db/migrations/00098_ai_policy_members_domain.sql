-- +goose Up
-- Domain baru `members` (member, poin, deposit): sebelumnya tool member_overview ikut domain `sales`. Agar perilaku tidak berubah,
-- semua kebijakan yang membuka `sales` ikut membuka `members`.
ALTER TABLE ai_policies DROP CONSTRAINT ai_policies_domains_valid;
ALTER TABLE ai_policies ADD CONSTRAINT ai_policies_domains_valid CHECK (domains IS NULL OR domains <@ ARRAY['sales','members','stock','debts','purchases','cash','accounting']::text[]);
UPDATE ai_policies SET domains = array_append(domains, 'members') WHERE domains IS NOT NULL AND 'sales' = ANY(domains) AND NOT ('members' = ANY(domains));

-- +goose Down
UPDATE ai_policies SET domains = array_remove(domains, 'members') WHERE domains IS NOT NULL;
ALTER TABLE ai_policies DROP CONSTRAINT ai_policies_domains_valid;
ALTER TABLE ai_policies ADD CONSTRAINT ai_policies_domains_valid CHECK (domains IS NULL OR domains <@ ARRAY['sales','stock','debts','purchases','cash','accounting']::text[]);
