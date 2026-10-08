import type { Messages } from '../../types.ts';

const receivables: Messages['receivables'] = {
  docTitle: 'Member Receivables | ACIRABA',
  title: 'Member Receivables',
  subtitle: 'Receivables from credit sales to members, in the outlets you can access. Payments may be made in instalments.',
  search: 'Search receipt no., member code or name…',
  loadFailed: 'Failed to load receivables.',
  empty: 'No receivables yet.',
  emptySearch: 'No receivable matches the filters.',
  loadMore: 'Load more',
  status: { open: 'Unpaid', overdue: 'Overdue', paid: 'Paid', all: 'All' },
  stat: { outstanding: 'Total receivable', overdue: 'Overdue', open: 'Unpaid receipts' },
  col: { docNo: 'Receipt No.', member: 'Member', date: 'Date', due: 'Due', amount: 'Receivable', paid: 'Paid', balance: 'Balance', status: 'Status', action: 'Action' },
  noDue: '—',
  pay: 'Pay',
  detail: 'Details',
  modal: {
    title: 'Receivable {doc}',
    info: 'Receivable details',
    history: 'Payment history',
    noPayments: 'No payments yet.',
    payTitle: 'Receive payment',
    method: 'Method',
    amount: 'Amount paid',
    ref: 'Reference no.',
    note: 'Note',
    full: 'Pay in full',
    save: 'Save payment',
    saving: 'Saving…',
    saved: 'Payment {doc} saved.',
    settled: 'This receivable is already settled.',
    receivedBy: 'Received by',
    fee: 'Method fee {fee}',
    afterPay: 'Balance after this payment: {balance}',
    close: 'Close'
  }
};

export default receivables;
