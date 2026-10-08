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
  },
  conversion: {
    docTitle: 'Unit Conversion | ACIRABA',
    title: 'Unit Conversion',
    subtitle: 'Move stock from one item to another at the active outlet (e.g. 1 Sack becomes 12 PCS). Both items keep their own stock; enter each quantity in that item’s base unit.',
    form: {
      title: 'New document',
      from: 'Source item (stock decreases)',
      to: 'Target item (stock increases)',
      pick: 'Search name, code, or barcode…',
      qty: 'Quantity',
      stock: 'Display stock: {qty} {unit}',
      note: 'Note (optional)',
      submit: 'Convert now',
      submitting: 'Processing…',
      summary: {
        title: 'Summary: what will happen',
        meaning: '{fromQty} {fromUnit} of {fromName} is converted into {toQty} {toUnit} of {toName}. In other words, 1 {fromUnit} of {fromName} counts as {ratio} {toUnit} of {toName}.',
        down: 'Stock of {name} decreases by {qty} {unit}',
        up: 'Stock of {name} increases by {qty} {unit}',
        change: '{before} → {after} {unit}',
        minus: 'Stock of {name} will go negative. This only works for items set to allow negative stock; otherwise the system rejects it as insufficient stock.',
        costNew: 'The cost of {name} will be recalculated from the source item’s cost, because it has no stock yet.',
        costKept: 'The cost of {name} stays the same.',
        final: 'A created document cannot be edited or deleted. If it is wrong, create the reverse document.'
      },
      hint: 'The target quantity is free-form, so a difference or loss (e.g. 1 sack yielding only 11 pcs) is captured directly.',
      done: 'Document {doc} created.',
      loadFailed: 'Failed to load data.'
    },
    history: {
      title: 'History',
      empty: 'No unit conversions at this outlet yet.',
      doc: 'Document',
      date: 'Time',
      move: 'Movement',
      cost: 'Resulting cost',
      costApplied: 'applied to the target item',
      costKept: 'target item cost unchanged',
      by: 'By'
    }
  },
  card: {
    docTitle: 'Stock Card | ACIRABA',
    title: 'Stock Card',
    subtitle: 'Stock in and out history of one item at the active outlet, with running balance.',
    item: 'Item',
    pickItem: 'Search name, code, or barcode…',
    period: 'Period',
    bucketAll: 'All buckets',
    prompt: 'Pick an item to see its stock card.',
    summary: { opening: 'Opening balance', in: 'In', out: 'Out', closing: 'Closing balance' },
    col: { time: 'Time', type: 'Type', doc: 'Document', bucket: 'Bucket', in: 'In', out: 'Out', balance: 'Balance', by: 'By' },
    bucket: { display: 'Display', warehouse: 'Warehouse', returns: 'Returns' },
    type: {
      OPENING: 'Opening balance',
      SALE: 'Sale',
      SALE_VOID: 'Sale void',
      SALE_RETURN: 'Sales return',
      PURCHASE: 'Purchase',
      PURCHASE_RETURN: 'Purchase return',
      OPNAME: 'Stock count',
      TRANSFER_OUT: 'Transfer out',
      TRANSFER_IN: 'Transfer in',
      UNIT_CONVERSION: 'Unit split',
      ADJUSTMENT: 'Adjustment'
    },
    empty: 'No stock movements in this period.',
    loadMore: 'Load more',
    preset: { today: 'Today', days7: '7 days', days30: '30 days', month: 'This month' },
    count: '{count} movements in this period',
    flowHint: 'Opening + in − out = closing',
    loadFailed: 'Failed to load the stock card.',
    inUnit: 'Base unit: {unit}'
  }
};

export default stock;
