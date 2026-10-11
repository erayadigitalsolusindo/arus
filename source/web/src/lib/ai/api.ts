// Klien API Asisten AI (HalloArus). Asisten hanya-baca di server; klien hanya mengirim teks percakapan.
import { api } from '#lib/api/client.ts';

export type FreeInfo = { enabled: boolean; limit: number; remaining: number; model: string };
export type Connection = { id: string; client: string; user_name: string; created_at: string; last_used_at?: string; own: boolean };
export type Provider = 'anthropic' | 'openai' | 'gemini' | 'qwen';
export type ProviderInfo = { id: Provider; configured: boolean; key_hint?: string; model: string; models: string[] };
export type AiSettings = { configured: boolean; active: boolean; free: FreeInfo; connector_url?: string; provider: Provider; providers: ProviderInfo[] };
export type ChatMessage = { role: 'user' | 'assistant'; content: string };
export type ToolCall = { name: string; input?: Record<string, unknown>; error?: boolean };
export type ChatResult = { reply: string; provider: Provider; model: string; tools_used: ToolCall[]; usage: { input_tokens: number; output_tokens: number }; free?: FreeInfo };

export const ai = {
  settings: () => api<AiSettings>('/ai/settings'),
  save: (input: { provider: Provider; api_key?: string; model: string }) => api<AiSettings>('/ai/settings', { method: 'PUT', body: JSON.stringify(input) }),
  removeKey: (provider: Provider) => api<null>(`/ai/settings/key?provider=${provider}`, { method: 'DELETE' }),
  connections: () => api<{ connections: Connection[] }>('/ai/connections').then((r) => r.connections),
  revokeConnection: (id: string) => api<null>(`/ai/connections/${id}`, { method: 'DELETE' }),
  clientInfo: (q: URLSearchParams) => api<{ name: string; host: string }>(`/oauth/client-info?${q}`),
  approve: (body: Record<string, string>) => api<{ redirect_url: string }>('/oauth/authorize/approve', { method: 'POST', body: JSON.stringify(body) }),
  chat: (messages: ChatMessage[], signal?: AbortSignal) => api<ChatResult>('/ai/chat', { method: 'POST', body: JSON.stringify({ messages }), signal })
};
