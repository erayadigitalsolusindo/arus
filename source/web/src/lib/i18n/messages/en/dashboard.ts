import type { Messages } from '../../types.ts';

const dashboard: Messages['dashboard'] = {
  docTitle: 'Dashboard · ACIRABA',
  title: 'Overview',
  badge: 'Phase 0 · Foundation',
  welcome: 'Welcome to ACIRABA',
  intro: 'The application skeleton is ready. Sales, stock, and receivable figures will appear here once the related modules are built.',
  exportReport: 'Export Report',
  reload: 'Reload',
  devVersion: 'Development version',
  kpi: {
    salesToday: "Today's Sales",
    monthlyTarget: 'Monthly target',
    profitShare: 'Profit contribution',
    receipts: 'Receipts',
    dailyAverage: 'Daily average',
    peakHour: 'Peak hour',
    lowStock: 'Low Stock',
    activeItems: 'Active items',
    negativeStock: 'Negative stock',
    overdue: 'Overdue Receivables',
    totalReceivable: 'Total receivables',
    due7Days: 'Due in 7 days'
  }
};

export default dashboard;
