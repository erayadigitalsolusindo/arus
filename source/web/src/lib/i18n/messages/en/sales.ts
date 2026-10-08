import type { Messages } from '../../types.ts';

const sales: Messages['sales'] = {
  docTitle: 'Sales List | ACIRABA',
  title: 'Sales List',
  subtitle: 'Every receipt from every cashier in the selected outlet, with a full discount breakdown.',
  search: 'Search receipt no., member or cashier…',
  period: 'Period',
  loadFailed: 'Could not load the sales list.',
  empty: 'No sales in this period yet.',
  emptySearch: 'No receipts match the filters.',
  loadMore: 'Load more',
  status: { label: 'Status', all: 'All statuses', completed: 'Completed', void: 'Voided' },
  method: { label: 'Payment method', all: 'All methods', cash: 'Cash', debit: 'Debit / QRIS', credit_card: 'Credit card', ewallet: 'E-money', transfer: 'Transfer' },
  col: {
    time: 'Time',
    docNo: 'Receipt No.',
    outlet: 'Outlet',
    cashier: 'Cashier',
    member: 'Member',
    items: 'Items',
    subtotal: 'Subtotal',
    discount: 'Discounts',
    tax: 'Tax & fees',
    total: 'Total',
    methods: 'Payments',
    cost: 'Cost',
    profit: 'Gross profit'
  },
  badge: {
    manual: 'Manual',
    line: 'Per line',
    points: 'Points',
    override: 'Price changed',
    voidDoc: 'Voided'
  },
  tip: {
    manual: 'Manual receipt-level discount',
    line: 'Discount on item lines',
    voucher: 'Coupon {code}',
    points: 'Redeemed {points} points',
    override: '{count} line(s) had the price changed (PIN-approved)'
  },
  summary: {
    count: 'Receipts',
    voided: '{count} voided',
    total: 'Total sales',
    discount: 'Total discounts',
    cost: 'Total cost',
    profit: 'Gross profit',
    methods: 'By payment method',
    average: 'Average {amount} / receipt',
    discountShare: '{pct}% of gross sales',
    margin: 'Margin {pct}%'
  },
  showing: 'Showing {shown} of {total} receipts',
  detail: {
    open: 'View receipt details',
    title: 'Receipt Details',
    loadFailed: 'Could not load the receipt details.',
    tabs: { items: 'Items', discounts: 'Discounts', payments: 'Payments', stock: 'Stock', history: 'History' },
    info: { time: 'Time', outlet: 'Outlet', cashier: 'Cashier', member: 'Member', salesperson: 'Salesperson', approvedBy: 'Approved by', note: 'Receipt note' },
    stat: { total: 'Total', paid: 'Paid', change: 'Change', profit: 'Gross profit', cost: 'Cost', items: '{count} lines · {qty} units' },
    items: {
      no: '#',
      item: 'Item',
      qty: 'Qty',
      listPrice: 'List price',
      price: 'Sold at',
      discount: 'Discount',
      total: 'Amount',
      cost: 'Cost/unit',
      profit: 'Profit',
      overridden: 'Price changed',
      baseQty: '= {qty} base units',
      lineNote: 'Note'
    },
    discounts: {
      calc: 'Receipt calculation',
      subtotal: 'Subtotal (after line discounts)',
      lineDiscount: 'Line discounts (already in subtotal)',
      manual: 'Manual receipt discount',
      voucher: 'Coupon',
      points: 'Points redeemed',
      pointsDetail: '{points} points redeemed',
      afterDiscount: 'Taxable base',
      taxStore: 'Store tax ({pct}%)',
      taxGov: 'Government tax ({pct}%)',
      otherCost: 'Other fees',
      unnamedCost: 'Unnamed fee',
      total: 'Total',
      sources: 'Discount sources',
      none: 'No discounts on this receipt.',
      kindPercent: '{value}% off',
      kindAmount: 'Fixed amount',
      earned: 'Points earned on this receipt',
      lineDiscountItems: 'Line discounts'
    },
    payments: { method: 'Method', amount: 'Amount', ref: 'Reference no.', paid: 'Total received', change: 'Change', net: 'Net payment', empty: 'No payments.' },
    stock: {
      intro: 'How this receipt moved stock, in each item’s base unit. Edits or voids will appear here as new movements.',
      time: 'Time',
      item: 'Item',
      bucket: 'Stock location',
      change: 'Change',
      after: 'Balance after',
      type: 'Type',
      bucketNames: { display: 'Display', warehouse: 'Warehouse', returns: 'Returns' },
      types: { SALE: 'Sale', SALE_VOID: 'Receipt void', SALE_RETURN: 'Sales return' },
      empty: 'This receipt did not move stock (services only).'
    },
    history: { intro: 'Every audit record for this receipt, earliest first.', empty: 'No records yet.', by: 'by {actor}', approver: 'Approved by {name}' }
  }
};

export default sales;
