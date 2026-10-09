import type { Messages } from '../../types.ts';

const payables: Messages['payables'] = {
  docTitle: 'Supplier Payables | ACIRABA',
  title: 'Supplier Payables',
  subtitle: 'Payables from credit purchases at the branches you can access. Payments may be made in instalments.',
  search: 'Search purchase no., invoice no., or supplier…',
  loadFailed: 'Failed to load payables.',
  empty: 'No payables yet.',
  emptySearch: 'No payables match the filter.',
  loadMore: 'Load more',
  status: { open: 'Unpaid', overdue: 'Overdue', paid: 'Paid', all: 'All' },
  stat: { outstanding: 'Total payable', overdue: 'Overdue', open: 'Unpaid invoices' },
  aging: { title: 'Payable aging', current: 'Not yet due', d1_30: '1–30 days overdue', d31_60: '31–60 days overdue', d60_plus: '> 60 days overdue' },
  col: { docNo: 'Purchase No.', invoice: 'Invoice No.', supplier: 'Supplier', date: 'Date', due: 'Due date', amount: 'Payable', paid: 'Paid', returned: 'Offset by returns', balance: 'Balance', status: 'Status', action: 'Action' },
  noDue: '—',
  pay: 'Pay',
  detail: 'Details',
  modal: {
    title: 'Payable {doc}',
    history: 'Payment history',
    noPayments: 'No payments yet.',
    payTitle: 'Pay payable',
    method: 'Method',
    amount: 'Amount paid',
    ref: 'Reference no.',
    note: 'Note',
    full: 'Pay in full',
    save: 'Save payment',
    saving: 'Saving…',
    saved: 'Payment {doc} saved.',
    settled: 'This payable is settled.',
    paidBy: 'Paid by',
    afterPay: 'Balance after this payment: {balance}',
    close: 'Close'
  },
  history: {
    docTitle: 'Buying Price History | ACIRABA',
    title: 'Buying Price History',
    subtitle: 'Buying price of items from every active purchase at the branches you can access, newest first.',
    search: 'Search item, code, purchase no., or supplier…',
    period: 'Period',
    loadFailed: 'Failed to load buying price history.',
    empty: 'No purchases in this period.',
    loadMore: 'Load more',
    count: '{count} rows loaded',
    col: { date: 'Date', item: 'Item', supplier: 'Supplier', qty: 'Qty', price: 'Buying price', change: 'Change', discount: 'Discount', cost: 'Line cost', doc: 'Purchase No.' },
    first: 'First',
    preset: { today: 'Today', days7: '7 days', days30: '30 days', month: 'This month' }
  }
};

export default payables;
