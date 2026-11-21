import type { Messages } from '../../types.ts';

const danger: Messages['danger'] = {
  docTitle: 'Danger Zone',
  title: 'Danger Zone',
  lead: 'Actions here delete data PERMANENTLY and cannot be undone. Only the Owner can run them. Take a backup first if unsure.',
  untouched: 'Master data (items, members, suppliers, accounts, etc.) is not deleted.',
  sales: 'Delete sales transactions',
  salesDesc: 'Deletes all sales receipts with their returns, receivables created by receipts, EraPay charges, daily summaries, and the automatic accounting journals of sales. Also deleted to stay in sync: sales stock movements (stock is restored), the whole member points and deposit ledgers (balances become zero), and all cashier shifts.',
  purchasing: 'Delete payables, receivables & purchases',
  purchasingDesc: 'Deletes all purchase receipts, purchase returns, all payables and receivables with their payments/settlements, and their automatic accounting journals. Also deleted to stay in sync: purchase stock movements (stock is reduced) and the whole supplier credit ledger (balance becomes zero).',
  methods: 'Reset payment methods',
  methodsDesc: 'Deletes ALL transactions above (receipts are attached to payment methods), deletes every method except Cash, Member Deposit and Supplier Credit, then restores those three to their defaults. The EraPay method is recreated if its API key is still installed.',
  run: 'Delete…',
  cancel: 'Cancel',
  confirmTitle: 'Confirm: {action}',
  typeWord: 'Type {word} to continue',
  password: 'Your password',
  go: 'Delete permanently',
  working: 'Deleting…',
  done: 'Done. Rows deleted: {n}.',
};

export default danger;
