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
  receipt: {
    title: 'Receipt',
    hint: 'Printed on the cashier receipt (58 mm paper, about 32 characters per line). Leave empty if not needed.',
    address: 'Address',
    phone: 'Phone',
    header: 'Receipt header text',
    footer: 'Receipt footer text'
  },
  geo: {
    title: 'Location (GPS)',
    hint: 'The outlet position, so the ACIRABA team knows where your outlets are. Press the button while you are at the outlet.',
    lat: 'Latitude',
    lng: 'Longitude',
    use: 'Use my location',
    locating: 'Finding location…',
    clear: 'Clear location',
    open: 'View on map',
    denied: 'Location permission denied. Allow location access in your browser and try again.',
    unavailable: 'Location is not available on this device.',
    invalid: 'Fill both latitude (−90 to 90) and longitude (−180 to 180), or leave both empty.'
  },
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
