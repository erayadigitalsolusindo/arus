import type { Messages } from '../../types.ts';

const onboarding: Messages['onboarding'] = {
  docTitle: 'Initial setup | ACIRABA',
  title: 'Set up your ARUS',
  subtitle: 'A few quick steps to get the app ready to use.',
  skip: 'Skip setup',
  back: 'Back',
  next: 'Next',
  stepOf: 'Step {n} of {total}',
  steps: { email: 'Email', coa: 'Accounting', ready: 'Readiness', done: 'Done' },
  email: {
    title: 'Confirm your email',
    text: 'We sent a verification link to {email}. Open it on any device; this page will update by itself.',
    verified: 'Email {email} is verified.',
    waiting: 'Waiting for confirmation…',
    resend: 'Resend email',
    sending: 'Sending…',
    sent: 'Email sent. Check your inbox and spam folder.',
    later: 'Continue for now, verify later'
  },
  coa: {
    title: 'Install the accounting template',
    text: 'A ready-made retail chart of accounts (assets, liabilities, equity, revenue, COGS, expenses) with automatic journal mappings and cash/bank accounts for your payment methods.',
    point1: 'Sales, purchase, stock and cash journals are created automatically',
    point2: 'Income statement, balance sheet and ledger fill in right away',
    point3: 'Accounts and mappings can be changed at any time',
    install: 'Install retail template',
    installing: 'Installing…',
    installed: 'Accounting template installed ({count} accounts).',
    failed: 'The template could not be installed. Try again or set it up under Accounting.',
    noPermission: 'Only users allowed to create accounts can install the template.'
  },
  ready: {
    title: 'Make sure the store is ready',
    text: 'These checks come from your current data.',
    outlet: 'First outlet created',
    methods: 'Payment methods linked to cash/bank accounts',
    methodsFix: '{count} methods are not linked to an account. Set them up under Payment Methods.',
    coa: 'Chart of accounts installed',
    items: 'First item added',
    itemsHint: 'Add items so the cashier can sell. You can do it now or later.',
    addItems: 'Add items',
    fixMethods: 'Open Payment Methods'
  },
  done: {
    title: 'All set',
    text: 'ARUS is set up. You can change everything at any time from the menu.',
    start: 'Start using ARUS',
    saving: 'Saving…',
    failed: 'Could not save. Try again.'
  }
};

export default onboarding;
