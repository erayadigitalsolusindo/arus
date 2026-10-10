import type { Messages } from '../../types.ts';

const deposits: Messages['deposits'] = {
  title: 'Deposit',
  balance: 'Deposit balance',
  hint: 'Money the member keeps with the store. It can pay sales and receivables; sales return refunds can go here.',
  topup: 'Top up',
  withdraw: 'Withdraw',
  empty: 'No deposit history yet.',
  loadMore: 'Load more',
  form: {
    topupTitle: 'Top up deposit',
    withdrawTitle: 'Withdraw deposit',
    topupHint: 'The member leaves money with the store (money comes into the active outlet).',
    withdrawHint: 'The member takes some or all of the deposit back (money leaves the active outlet).',
    amount: 'Amount',
    method: 'Via method',
    ref: 'Reference no.',
    refHint: 'Required for transfer, debit, card, and e-wallet.',
    note: 'Note (optional)',
    save: 'Save',
    saving: 'Saving…',
    saved: 'Document {doc} saved.',
    cancel: 'Cancel'
  },
  history: { time: 'Time', kind: 'Type', doc: 'Document', amount: 'Amount', balance: 'Balance', by: 'By' },
  kind: {
    TOPUP: 'Top-up',
    WITHDRAW: 'Withdrawal',
    SALE_PAYMENT: 'Sale payment',
    SALE_REVERSAL: 'Sale voided/edited',
    RECEIVABLE_PAYMENT: 'Receivable payment',
    SALE_RETURN: 'Return refund',
    SALE_RETURN_VOID: 'Return voided',
    PURCHASE_RETURN: 'Purchase return refund',
    PURCHASE_RETURN_VOID: 'Purchase return voided',
    PAYABLE_PAYMENT: 'Payable payment',
    CASH_OUT: 'Cash-out'
  }
};

export default deposits;
