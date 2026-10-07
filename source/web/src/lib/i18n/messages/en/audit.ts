import type { Messages } from '../../types.ts';

const audit: Messages['audit'] = {
  docTitle: 'Audit Log | ACIRABA',
  title: 'Audit Log',
  subtitle: 'A record of who changed what. Read-only; entries cannot be edited or deleted.',
  entity: 'Object',
  action: 'Action',
  from: 'From date',
  to: 'To date',
  all: 'All',
  apply: 'Apply',
  reset: 'Reset',
  time: 'Time',
  actor: 'Actor',
  details: 'Details',
  ip: 'IP',
  system: 'System',
  loadMore: 'Load more',
  loading: 'Loading…',
  empty: 'No entries for this filter.',
  loadFailed: 'Failed to load the audit log.',
  entities: { user: 'User', role: 'Role', outlet: 'Outlet', session: 'Session', tenant: 'Tenant' },
  actions: {
    auth_register: 'Registered the business',
    auth_login: 'Signed in',
    auth_password_reset: 'Password reset (forgot password)',
    auth_password_change: 'Changed password',
    auth_email_verified: 'Verified email',
    auth_outlet_switch: 'Switched outlet',
    auth_terms_accepted: 'Accepted terms of service',
    user_create: 'Added a user',
    user_update: 'Edited a user',
    user_password_reset: 'Reset a user password',
    role_create: 'Added a role',
    role_update: 'Edited a role',
    role_delete: 'Deleted a role',
    outlet_create: 'Added an outlet',
    outlet_update: 'Edited an outlet',
    platform_impersonate: 'Platform Admin viewed tenant data (read-only)',
    platform_tenant_status: 'Platform Admin changed the tenant status'
  }
};

export default audit;
