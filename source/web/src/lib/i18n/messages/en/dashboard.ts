import type { Messages } from '../../types.ts';

const dashboard: Messages['dashboard'] = {
  docTitle: 'Dashboard · ACIRABA',
  title: 'Store Overview',
  refresh: 'Refresh',
  updatedAt: 'Updated {time}',
  loadError: 'The overview could not be loaded. Check your connection and try again.',
  retry: 'Try again',
  noAccess: 'Your account has no permission to view the overview. Contact the store owner.',
  scope: { outlet: 'Branch {name}', all: 'All branches ({count})' },
  verdict: {
    ok: { title: 'Your store is doing fine', text: 'All checks passed. Nothing needs your attention right now.' },
    warn: { title: 'Some things need a look', text: 'The store is running, but {count} item(s) deserve a check.' },
    bad: { title: 'Something needs attention now', text: 'There is a serious problem in {count} item(s). See the list below.' }
  },
  checksTitle: 'What was checked',
  checksSub: 'Green is fine, yellow needs a look, red needs action now.',
  open: 'Open',
  check: {
    sales_pace: {
      ok: 'Sales today are {pct} compared with usual at this hour — normal.',
      warn: 'Sales today are {pct} compared with usual at this hour. Make sure the cashier is open and the store is as busy as usual.'
    },
    sales_pace_nodata: { ok: 'Not enough 4-week history to compare sales yet — it will appear automatically.' },
    voids: {
      ok: 'Receipts cancelled today: {count} — normal.',
      warn: '{count} receipts were cancelled today, more than usual. Check the reasons in the Sales List.',
      bad: '{count} receipts were cancelled today — a lot. Check the Sales List right away.'
    },
    shift_open_long: {
      ok: 'No cashier shift was left open.',
      warn: '{count} cashier shift(s) have been open for over 18 hours — probably forgotten.'
    },
    shift_diff: {
      ok: 'Cash in the drawer matched the records over the last 7 days.',
      warn: '{count} shift(s) in the last 7 days had a cash difference (total {amount}).'
    },
    stock_low_unset: { ok: 'No minimum stock level is set yet — set it on the item form to get low-stock warnings.' },
    stock_low: {
      ok: 'No item is running low (below its minimum level).',
      warn: '{count} item(s) are running low — below their minimum level, time to restock.'
    },
    stock_negative: {
      ok: 'No item has negative stock.',
      warn: '{count} item(s) have negative stock — usually sold before the incoming goods were recorded.'
    },
    receivable_overdue: {
      ok: 'No member receivable is overdue.',
      warn: 'Member receivables are overdue by {amount}.',
      bad: 'More than half of member receivables ({amount}) are overdue.'
    },
    payable_overdue: {
      ok: 'No supplier payable is overdue.',
      warn: 'Supplier payables are overdue by {amount}.',
      bad: 'Supplier payables are overdue by more than 30 days ({amount}).'
    }
  },
  kpi: {
    salesToday: 'Sales today',
    yesterday: 'Yesterday, full day: {amount}',
    vsUsual: '{pct} vs usual',
    vsUsualNone: 'No comparison yet',
    vsUsualHint: 'Compared with the average of the same weekday over the last 4 weeks, up to this hour.',
    receipts: 'Receipts',
    average: 'Average {amount} per receipt',
    cancelled: '{count} cancelled',
    profit: 'Gross profit today',
    profitHint: 'After cost and returns, before tax and other charges.',
    week: 'Last 7 days',
    month: 'This month',
    monthVs: '{pct} vs last month',
    monthVsHint: 'Compared with last month on the same date and hour: {amount}.',
    monthVsNone: 'No sales last month',
    returns: 'Returns today: {count} ({amount})'
  },
  chart: {
    trend: 'Daily sales',
    days: '{count} days',
    hourly: 'Sales by hour today',
    peak: 'Busiest at {hour}',
    today: 'today',
    receiptsCount: '{count} receipts',
    noSales: 'No sales in this period yet. The chart fills in once the cashier starts selling.'
  },
  top: { title: 'Best sellers', sub: 'Last 7 days, by revenue', qty: '{qty} sold', empty: 'Nothing sold yet.' },
  stock: {
    title: 'Stock health',
    tracked: 'Items with a balance',
    empty: 'Out of stock',
    negative: 'Negative stock',
    low: 'Running low',
    lowList: 'Lowest (stock / minimum level)',
    lowUnset: 'Set "Minimum stock level" on the item form to see low-stock items here.',
    value: 'Inventory value',
    valueHint: 'Quantity × average cost.',
    all: 'Open item list',
    negativeList: 'Largest negative stock'
  },
  receivable: { title: 'Member receivables', outstanding: 'Unpaid', overdue: 'Overdue', open: '{count} unpaid receipts', all: 'Open receivables' },
  payable: { title: 'Supplier payables', outstanding: 'Unpaid', overdue: 'Overdue', open: '{count} unpaid invoices', all: 'Open payables' },
  branches: {
    title: 'Branch comparison',
    sub: 'Sales and condition of each branch right now',
    outlet: 'Branch',
    today: 'Today',
    receipts: 'Receipts',
    yesterday: 'Yesterday',
    week: '7 days',
    stock: 'Stock',
    shifts: 'Open shifts',
    stockOk: 'Fine',
    negative: '{count} negative',
    low: '{count} low',
    empty: '{count} empty'
  },
  wide: 'Receivables and payables always cover every branch you can access.',
  shifts: {
    title: 'Cash & cashier shifts',
    running: 'Shifts in progress',
    none: 'No shift is open right now.',
    since: 'since {time}',
    long: 'too long',
    diff: 'Cash difference, last 7 days',
    noDiff: 'No difference',
    closed: '{count} shifts closed this week',
    all: 'Open shift list'
  }
};
export default dashboard;
