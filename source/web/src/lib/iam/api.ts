// Klien API untuk role dan pengguna (backend/internal/iam). Penegakan izin ada di server; UI hanya menyaring tampilan.
import { api, type Permissions } from '#lib/api/client.ts';

export type ModuleDef = { id: string; actions: string[] };

export type Role = {
  id: string;
  name: string;
  permissions: Permissions;
  is_system: boolean;
  user_count: number;
};

export type User = {
  id: string;
  name: string;
  email: string;
  phone: string;
  active: boolean;
  role_id: string;
  role_name: string;
  last_login_at: string | null;
  created_at: string;
};

const json = (body: unknown) => JSON.stringify(body);

export const iam = {
  registry: () => api<{ modules: ModuleDef[] }>('/iam/registry').then((r) => r.modules),

  roles: () => api<{ roles: Role[] }>('/iam/roles').then((r) => r.roles),
  assignableRoles: () => api<{ roles: Role[] }>('/iam/assignable-roles').then((r) => r.roles),
  createRole: (name: string, permissions: Record<string, string[]>) => api<Role>('/iam/roles', { method: 'POST', body: json({ name, permissions }) }),
  updateRole: (id: string, name: string, permissions: Record<string, string[]>) =>
    api<Role>(`/iam/roles/${id}`, { method: 'PUT', body: json({ name, permissions }) }),
  deleteRole: (id: string) => api<null>(`/iam/roles/${id}`, { method: 'DELETE' }),

  users: () => api<{ users: User[] }>('/iam/users').then((r) => r.users),
  createUser: (u: { name: string; email: string; phone: string; password: string; role_id: string }) =>
    api<User>('/iam/users', { method: 'POST', body: json(u) }),
  updateUser: (id: string, u: { name: string; phone: string; role_id: string; active: boolean }) =>
    api<User>(`/iam/users/${id}`, { method: 'PUT', body: json(u) }),
  resetPassword: (id: string, password: string) => api<null>(`/iam/users/${id}/password`, { method: 'PUT', body: json({ password }) })
};
