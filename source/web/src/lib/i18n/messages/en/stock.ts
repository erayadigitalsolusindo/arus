import type { Messages } from '../../types.ts';

// Stock (opening balances, etc.).
const stock: Messages['stock'] = {
  opening: {
    docTitle: 'Opening Stock | ACIRABA',
    title: 'Opening Stock',
    subtitle: 'Enter starting stock per item for the active outlet. Type a quantity, then press Enter or move to another field to save.',
    search: 'Search name, code, or barcode…',
    col: { code: 'Code', name: 'Name', unit: 'Unit' },
    bucket: { display: 'Display', warehouse: 'Warehouse', returns: 'Returns' },
    saved: 'Saved',
    saving: 'Saving…',
    empty: 'No stocked items yet (goods, not services).',
    emptySearch: 'No matching items.',
    loadFailed: 'Failed to load data.',
    range: 'Showing {from}–{to} of {total}',
    prev: 'Previous',
    next: 'Next',
    status: {
      open: 'Opening balances can still be entered and changed.',
      lockedOn: 'Opening balances are locked. Operations started: {date}. Further stock changes go through stock count.'
    },
    lock: {
      button: 'Lock & start operations',
      title: 'Lock opening balances',
      body: 'Once locked, this outlet’s opening balances can no longer be changed; stock corrections only through stock count. Make sure all quantities are correct.',
      date: 'Operations start date',
      confirm: 'Lock now',
      cancel: 'Cancel',
      done: 'Opening balances locked. The outlet is ready to trade.'
    }
  }
};

export default stock;
