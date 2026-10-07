import type { Messages } from '../../types.ts';

const shell: Messages['shell'] = {
  openMenu: 'Open menu',
  closeMenu: 'Close menu',
  expandSidebar: 'Expand sidebar',
  collapseSidebar: 'Collapse sidebar',
  mainNav: 'Main navigation',
  sidebar: 'Sidebar',
  search: 'Search items, receipts, customers…',
  quickCreate: 'Quick Create',
  quick: {
    newSale: 'New Sale',
    newItem: 'New Item',
    newCustomer: 'Add Customer',
    newPurchase: 'New Purchase'
  },
  notifications: 'Notifications',
  noNotifications: 'No notifications yet.',
  notSignedIn: 'Not signed in',
  guest: 'Guest',
  authPhase: 'Auth: Phase 2',
  profile: 'Profile',
  settings: 'Settings',
  signOut: 'Sign out',
  server: {
    checking: 'Checking server…',
    online: 'Server online',
    degraded: 'Server degraded',
    offline: 'Server unreachable'
  }
};

export default shell;
