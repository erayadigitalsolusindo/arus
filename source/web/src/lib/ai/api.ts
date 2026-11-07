// Klien API Asisten AI (HalloArus). Klien mengirim teks percakapan (+ foto nota di pesan terakhir). AI tidak menulis data:
// ia hanya mengusulkan aksi (PendingAction) yang baru dijalankan server setelah pengguna menyetujui lewat confirmAction.
import { api } from '#lib/api/client.ts';

export type FreeInfo = { enabled: boolean; limit: number; remaining: number; model: string; arus?: boolean };
export type Connection = { id: string; client: string; user_name: string; created_at: string; last_used_at?: string; own: boolean };
/** Penyedia bawaan ('anthropic'|'openai'|'gemini'|'qwen') atau penyedia kustom tenant ('custom_xxxxxxxx'). */
export type Provider = string;
/** Penyedia kustom (OpenAI-compatible): `label` + `base_url` diisi tenant, `model` teks bebas (models kosong). */
export type ProviderInfo = { id: Provider; configured: boolean; key_hint?: string; model: string; models: string[]; custom?: boolean; label?: string; base_url?: string };
export type CustomInput = { label: string; base_url: string; model: string; api_key?: string };
/** Riwayat percakapan: `available` = server punya kunci enkripsi, `enabled` = saklar tenant menyala, `days` = masa simpan. */
export type HistoryInfo = { available: boolean; enabled: boolean; days: number };
/** Batasan dari operator platform; menang atas izin role. */
export type AiPolicy = {
  enabled: boolean;
  domains: string[];
  allow_write: boolean;
  allow_cost: boolean;
  allow_byok: boolean;
  allow_custom: boolean;
  allow_images: boolean;
  allow_history: boolean;
  allow_mcp: boolean;
  chat_limit: number;
  max_range_days: number;
};
export type AiSettings = { configured: boolean; active: boolean; free: FreeInfo; connector_url?: string; provider: Provider; providers: ProviderInfo[]; history: HistoryInfo; policy: AiPolicy };
export type ChatImage = { media_type: string; data: string };
export type ChatMessage = { role: 'user' | 'assistant'; content: string; images?: ChatImage[] };
export type ActionKind = 'petty_cash_add' | 'petty_cash_void';
/** Usulan aksi tulis dari AI; `fields` dibuat server (bukan teks model): direction, amount, category, note, created. */
export type PendingAction = { id: string; kind: ActionKind; needs_pin: boolean; expires_at: string; fields: Record<string, string> };
export type ToolCall = { name: string; input?: Record<string, unknown>; error?: boolean };
export type ChatResult = { reply: string; provider: Provider; model: string; tools_used: ToolCall[]; usage: { input_tokens: number; output_tokens: number }; free?: FreeInfo; actions?: PendingAction[]; conversation_id?: string };
export type ConversationSummary = { id: string; title: string; msg_count: number; updated_at: string };
export type ConversationPage = { items: ConversationSummary[]; next?: string };
export type ActionStatus = 'pending' | 'done' | 'cancelled' | 'expired';
export type StoredMessage = { role: 'user' | 'assistant'; content: string; tools?: string[]; actions?: (PendingAction & { status: ActionStatus })[]; had_images?: boolean; created_at: string };
export type Conversation = ConversationSummary & { messages: StoredMessage[] };

// Server lama (belum dimuat ulang saat web sudah di-deploy) belum mengirim `history`: anggap riwayat tidak tersedia.
const openPolicy: AiPolicy = { enabled: true, domains: [], allow_write: true, allow_cost: true, allow_byok: true, allow_custom: true, allow_images: true, allow_history: true, allow_mcp: true, chat_limit: 20, max_range_days: 366 };
const withHistory = (s: AiSettings): AiSettings => ({ ...s, history: s.history ?? { available: false, enabled: false, days: 90 }, policy: s.policy ?? openPolicy });

export const ai = {
  settings: () => api<AiSettings>('/ai/settings').then(withHistory),
  save: (input: { provider: Provider; api_key?: string; model: string }) => api<AiSettings>('/ai/settings', { method: 'PUT', body: JSON.stringify(input) }).then(withHistory),
  /** Tambah (tanpa id) atau ubah penyedia kustom; server memverifikasi key ke endpoint-nya lalu menyimpannya terenkripsi. */
  saveCustom: (id: string | undefined, input: CustomInput) =>
    api<AiSettings>(id ? `/ai/custom/${id}` : '/ai/custom', { method: id ? 'PUT' : 'POST', body: JSON.stringify(input) }).then(withHistory),
  removeKey: (provider: Provider) => api<null>(`/ai/settings/key?provider=${provider}`, { method: 'DELETE' }),
  connections: () => api<{ connections: Connection[] }>('/ai/connections').then((r) => r.connections),
  revokeConnection: (id: string) => api<null>(`/ai/connections/${id}`, { method: 'DELETE' }),
  clientInfo: (q: URLSearchParams) => api<{ name: string; host: string }>(`/oauth/client-info?${q}`),
  approve: (body: Record<string, string>) => api<{ redirect_url: string }>('/oauth/authorize/approve', { method: 'POST', body: JSON.stringify(body) }),
  /** `conversation_id` = lanjutkan percakapan tersimpan; `temporary` = jangan simpan ke riwayat. */
  chat: (messages: ChatMessage[], signal?: AbortSignal, opts: { conversation_id?: string; temporary?: boolean } = {}) =>
    api<ChatResult>('/ai/chat', { method: 'POST', body: JSON.stringify({ messages, ...opts }), signal }),
  conversations: (cursor?: string) => api<ConversationPage>(`/ai/conversations?limit=30${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`),
  conversation: (id: string) => api<Conversation>(`/ai/conversations/${id}`),
  renameConversation: (id: string, title: string) => api<null>(`/ai/conversations/${id}`, { method: 'PATCH', body: JSON.stringify({ title }) }),
  deleteConversation: (id: string) => api<null>(`/ai/conversations/${id}`, { method: 'DELETE' }),
  deleteAllConversations: () => api<{ deleted: number }>('/ai/conversations', { method: 'DELETE' }),
  /** Saklar simpan riwayat milik tenant (izin ai_assistant.update). */
  setHistory: (save_history: boolean) => api<AiSettings>('/ai/prefs', { method: 'PUT', body: JSON.stringify({ save_history }) }).then(withHistory),
  /** Setujui usulan; `approval` (penyetuju + PIN) wajib bila `needs_pin`. PIN dikirim langsung ke server, tidak lewat percakapan. */
  confirmAction: (id: string, approval?: { user_id: string; pin: string }) =>
    api<{ id: string; kind: ActionKind }>(`/ai/actions/${id}/confirm`, { method: 'POST', body: JSON.stringify({ approval }) }),
  cancelAction: (id: string) => api<null>(`/ai/actions/${id}/cancel`, { method: 'POST' })
};
