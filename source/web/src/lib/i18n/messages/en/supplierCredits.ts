import type { Messages } from '../../types.ts';

const supplierCredits: Messages['supplierCredits'] = {
  docTitle: 'Supplier Credits | ACIRABA',
  title: 'Supplier Credits',
  subtitle: 'Supplier money the store still holds, from purchase return refunds. It can pay payables or be cashed out by the supplier.',
  search: 'Search supplier…',
  onlyPositive: 'Only with a balance',
  col: { supplier: 'Supplier', balance: 'Credit balance' },
  empty: 'No supplier credits yet.',
  loadMore: 'Load more',
  loadFailed: 'Failed to load supplier credits.',
  balanceLine: 'Supplier credit balance: {amount}',
  over: 'exceeds the credit balance',
  detail: 'Details',
  cashOut: {
    action: 'Record cash-out',
    title: 'Supplier credit cash-out',
    hint: 'The supplier pays its remaining credit to the store (money comes into the active outlet).'
  }
};

export default supplierCredits;
