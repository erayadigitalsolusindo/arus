// Klien API audit log (hanya baca; catatan ditulis server di transaksi yang sama dengan perubahannya).
import { api } from '#lib/api/client.ts';

export type AuditItem = {
  id: number;
  outlet_id: string | null;
  actor_id: string | null;
  actor_name: string;
  action: string;
  entity: string;
  entity_id: string;
  details: Record<string, unknown>;
  ip: string;
  request_id: string;
  created_at: string;
};

export type AuditFilter = { entity?: string; action?: string; from?: string; to?: string; cursor?: string; limit?: number };

export const audit = {
  list(f: AuditFilter = {}) {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(f)) if (v !== undefined && v !== '') q.set(k, String(v));
    const qs = q.toString();
    return api<{ items: AuditItem[]; next_cursor: string }>(`/audit-log${qs ? `?${qs}` : ''}`);
  }
};
