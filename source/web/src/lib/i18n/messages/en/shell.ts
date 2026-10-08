import type { Messages } from '../../types.ts';

const shell: Messages['shell'] = {
  openMenu: 'Open menu',
  closeMenu: 'Close menu',
  expandSidebar: 'Expand sidebar',
  collapseSidebar: 'Collapse sidebar',
  tabs: 'Page tabs',
  closeTab: 'Close tab',
  unsaved: {
    title: 'Unsaved changes',
    body: 'This form has not been saved and its contents will be lost if you leave the page. Discard changes?',
    stay: 'Stay here',
    discard: 'Discard changes'
  },
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
  profile: 'Profile',
  settings: 'Settings',
  signOut: 'Sign out',
  currentOutlet: 'Current Outlet',
  allOutlets: 'All Outlets',
  allOutletsShort: 'all',
  switchOutlet: 'Switch outlet?',
  server: {
    checking: 'Checking server…',
    online: 'Server online',
    degraded: 'Server degraded',
    offline: 'Server unreachable'
  }
};

export default shell;
