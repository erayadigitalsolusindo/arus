import type { Messages } from '../../types.ts';

const shell: Messages['shell'] = {
  openMenu: 'Open menu',
  closeMenu: 'Close menu',
  expandSidebar: 'Expand sidebar',
  collapseSidebar: 'Collapse sidebar',
  tabs: 'Page tabs',
  closeTab: 'Close tab',
  tabMenu: {
    close: 'Close tab',
    closeOthers: 'Close other tabs',
    closeRight: 'Close tabs to the right',
    closeAll: 'Close all tabs'
  },
  unsaved: {
    title: 'Unsaved changes',
    body: 'This form has not been saved and its contents will be lost if you close the tab. Discard changes?',
    stay: 'Stay here',
    discard: 'Discard changes',
    restored: 'Your unsaved draft was restored. It is kept while this tab stays open; images/photos must be picked again.'
  },
  mainNav: 'Main navigation',
  sidebar: 'Sidebar',
  shortcuts: {
    title: 'FEATURE SHORTCUTS',
    outlet: 'OUTLET: {outlet}',
    welcome: 'Welcome!',
    welcomeBody: 'Thank you for trusting us with your business. We keep improving this system to help your business grow.',
    features: 'Features',
    empty: 'No shortcuts yet.',
    add: 'Add shortcut',
    edit: 'Edit shortcut',
    remove: 'Remove shortcut',
    name: 'Feature name',
    url: 'Feature URL',
    icon: 'Icon',
    upload: 'Upload icon',
    removeIcon: 'Remove icon',
    iconHint: 'PNG, JPG, or WebP up to 128 KB. Stored on the server and available across devices.',
    iconInvalid: 'Choose a PNG, JPG, or WebP image up to 128 KB.',
    urlInvalid: 'Enter a name and valid URL (app path or HTTP/HTTPS address).',
    limit: 'You can add up to 8 shortcuts.',
    loading: 'Loading shortcuts…',
    loadFailed: 'Could not load shortcuts from the server.',
    saveFailed: 'Could not save shortcuts to your account.',
    retry: 'Try again'
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
