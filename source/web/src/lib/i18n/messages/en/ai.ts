import type { Messages } from '../../types.ts';

const ai: Messages['ai'] = {
  docTitle: 'HalloArus | ACIRABA',
  title: 'HalloArus',
  subtitle: 'An AI assistant that reads your business data, draws conclusions and gives advice.',
  readOnly: 'Read-only',
  readOnlyHint: 'HalloArus cannot add, change or delete any data.',
  privacy: 'Summarised figures are sent to Anthropic (Claude) using your business\'s own API key. Customer data (names, phones, addresses) is never sent; customers appear only as "Pelanggan 1, 2, …".',
  settings: 'Settings',
  setup: {
    title: 'Connect Claude',
    intro: 'HalloArus uses your own Claude API key; usage is billed by Anthropic directly to your account. Create a key at console.anthropic.com and paste it below.',
    keyLabel: 'Claude API key',
    keyPlaceholder: 'sk-ant-…',
    keyReplaceHint: 'Leave empty to keep the current key.',
    keySaved: 'Key saved (ends with {hint}). It is stored encrypted and cannot be viewed again.',
    modelLabel: 'Model',
    save: 'Save',
    saving: 'Checking & saving…',
    saved: 'Settings saved.',
    removeKey: 'Remove key',
    removeConfirm: 'Remove the API key? HalloArus stops working until a new key is set.',
    needKey: 'Enter your Claude API key first.',
    notConfigured: 'HalloArus is not active yet. Ask the business owner to set a Claude API key on this page.',
    notConfiguredManage: 'Set a Claude API key to activate HalloArus.'
  },
  models: {
    'claude-sonnet-5-5': 'Claude Sonnet 5.5 (balanced, recommended)',
    'claude-opus-5-5': 'Claude Opus 5.5 (deeper analysis)',
    'claude-fable-5-1': 'Claude Fable 5.1 (most capable)',
    'claude-haiku-5-5': 'Claude Haiku 5.5 (fast & cheap)'
  },
  free: {
    banner: 'Free account: {remaining} of {limit} questions left today. Add your own API key for unlimited use.',
    exhausted: 'The free quota for today is used up (resets tomorrow). Add your own API key to continue without limits.',
    useOwn: 'Use my own key'
  },
  connector: {
    button: 'Claude connector',
    title: 'Connect to the Claude app',
    intro: 'Use your own Claude subscription (no API key): add ARUS as a connector in Claude, then ask about your business data right there. The connector is read-only, with the same limits as HalloArus.',
    s1: 'In Claude open Settings → Connectors → Add custom connector.',
    s2: 'Paste the address below and save.',
    s3: 'Click Connect; you are sent to ARUS to approve read access.',
    urlLabel: 'Connector address',
    copy: 'Copy',
    copied: 'Copied',
    listTitle: 'Active connectors',
    none: 'No connector is connected yet.',
    by: 'by {name}',
    lastUsed: 'last used {when}',
    never: 'never used',
    revoke: 'Revoke',
    revokeConfirm: 'Revoke this connector? Claude can no longer read ARUS data through this grant.',
    revoked: 'Connector access revoked.'
  },
  connect: {
    docTitle: 'Allow Claude | ACIRABA',
    title: 'Allow {name} to read ARUS data?',
    lead: 'You are signed in as {user}. {name} (returning to {host}) is asking permission to:',
    c1: 'Read summaries of sales, stock, receivables/payables, cash and accounting within your access rights.',
    c2: 'See customers only as "Pelanggan 1, 2, …" (names, phones and addresses are never sent).',
    c3: 'It cannot add, change or delete any data.',
    allow: 'Allow',
    deny: 'Deny',
    working: 'Redirecting…',
    invalid: 'The request is invalid or expired. Start again from the Claude app.',
    revokeHint: 'You can revoke access any time in HalloArus → Claude connector.'
  },
  empty: {
    title: 'Ask anything about your business data',
    hint: 'HalloArus reads sales, stock, receivables/payables, cash and accounting within your access rights, then summarises and advises.'
  },
  suggestions: {
    s1: 'How did the last 30 days of sales compare to the 30 days before?',
    s2: 'Which items sell best and earn the most this month?',
    s3: 'Which items are running low and need restocking now?',
    s4: 'Any receivables at risk of going bad? Who should I chase first?',
    s5: 'Are there cash shortages or many voided invoices at the cashiers?',
    s6: 'Which items are sitting unsold and should go on clearance?'
  },
  input: {
    placeholder: 'Type your question… (Enter to send, Shift+Enter for a new line)',
    send: 'Send',
    clear: 'New conversation',
    stop: 'Cancel'
  },
  thinking: 'Reading data & analysing…',
  you: 'You',
  assistant: 'HalloArus',
  dataRead: 'Data read',
  usage: '{input} tokens in · {output} tokens out',
  disclaimer: 'AI answers can be wrong; check important figures in the reports. Not financial, tax or legal advice.',
  tools: {
    list_outlets: 'Outlet list',
    sales_summary: 'Sales summary',
    top_items: 'Best / worst selling items',
    sales_by_category: 'Sales by category',
    returns_summary: 'Returns summary',
    member_overview: 'Customer overview (anonymised)',
    stock_overview: 'Inventory health',
    stock_items: 'Items needing attention',
    slow_moving_items: 'Slow-moving stock',
    stock_opname_summary: 'Stock count results',
    receivables_summary: 'Receivables & aging',
    payables_summary: 'Supplier payables',
    purchases_summary: 'Purchases summary',
    cash_shifts_summary: 'Cashier shift control',
    petty_cash_summary: 'Petty cash',
    accounting_balances: 'Account balances (ledger)'
  }
};

export default ai;
