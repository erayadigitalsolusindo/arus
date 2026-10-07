import type { Messages } from '../../types.ts';

const outlets: Messages['outlets'] = {
  docTitle: 'Outlets | ACIRABA',
  title: 'Outlets',
  subtitle: 'Manage the branches or stores of your business.',
  add: 'Add Outlet',
  edit: 'Edit Outlet',
  code: 'Code',
  codeHint: 'Lowercase letters, digits, - or _ (2–20 characters). Cannot be changed after creation.',
  name: 'Outlet name',
  timezone: 'Time zone',
  taxStore: 'Store tax (%)',
  taxGov: 'Government tax (%)',
  taxHint: 'A percentage from 0 to 100, up to 2 decimals.',
  status: 'Status',
  active: 'Active',
  inactive: 'Inactive',
  current: 'In use',
  save: 'Save',
  saving: 'Saving…',
  cancel: 'Cancel',
  close: 'Close',
  saved: 'Outlet saved.',
  created: 'Outlet added.',
  empty: 'No outlets yet.',
  loadFailed: 'Failed to load outlets.'
};

export default outlets;
