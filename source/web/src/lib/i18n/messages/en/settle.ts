import type { Messages } from '../../types.ts';

const settle: Messages['settle'] = {
  payable: { button: 'Pay by Supplier', title: 'Pay Supplier Payables', party: 'Supplier', partyHint: 'Search supplier…', doc: 'Purchase No.', empty: 'This supplier has no unpaid payables.' },
  receivable: { button: 'Receive by Member', title: 'Receive Member Payment', party: 'Member', partyHint: 'Search member…', doc: 'Sale No.', empty: 'This member has no unpaid receivables.' },
  mode: { auto: 'Automatic (oldest first)', manual: 'Pick invoices' },
  autoHelp: 'Enter the total amount. The oldest invoices are settled first; the last one may be partially paid.',
  manualHelp: 'Tick the invoices to pay. The amount defaults to the full balance and can be reduced (instalment).',
  outstanding: 'Total outstanding: {amount} ({count} invoices)',
  amount: 'Total amount paid',
  payAll: 'Pay all',
  selectAll: 'Select all',
  clear: 'Clear',
  col: { pick: 'Pick', date: 'Date', due: 'Due', balance: 'Balance', pay: 'Paying', after: 'Remaining' },
  preview: 'Allocation',
  previewEmpty: 'Enter an amount to see how it is split across invoices.',
  total: 'Total: {amount}',
  method: 'Method',
  ref: 'Reference no.',
  note: 'Note',
  save: 'Save payment',
  saving: 'Saving…',
  close: 'Close',
  done: 'Payment {doc} saved',
  doneDetail: '{count} invoices paid, total {amount}.',
  fee: 'Method fee {fee}'
};

export default settle;
