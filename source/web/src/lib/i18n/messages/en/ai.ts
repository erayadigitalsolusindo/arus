import type { Messages } from '../../types.ts';

const ai: Messages['ai'] = {
  docTitle: 'HalloArus | ACIRABA',
  title: 'HalloArus',
  subtitle: 'An AI assistant that reads your business data, draws conclusions and gives advice.',
  readOnly: 'Read-only',
  readOnlyHint: 'HalloArus cannot add, change or delete any data.',
  privacy: 'Summarised figures are sent to the AI provider you choose (Claude, ChatGPT, Gemini or Qwen) using your business\'s own API key (or the free ARUS account). Customer data (names, phones, addresses) is never sent; customers appear only as "Pelanggan 1, 2, …".',
  settings: 'Settings',
  setup: {
    title: 'Connect AI',
    intro: 'HalloArus uses your own API key; usage is billed by the AI provider directly to your account. Create a key at {console} and paste it below. Each provider\'s key is stored separately; only one is active.',
    providerLabel: 'AI provider',
    keyLabel: '{provider} API key',
    keyReplaceHint: 'Leave empty to keep the current key.',
    keySaved: 'Key saved (ends with {hint}). It is stored encrypted and cannot be viewed again.',
    keyNone: 'No key for this provider yet.',
    modelLabel: 'Model',
    save: 'Save & activate',
    saving: 'Checking & saving…',
    saved: 'Settings saved.',
    removeKey: 'Remove {provider} key',
    removeConfirm: 'Remove the {provider} API key? If it is the active provider, HalloArus switches to another provider that has a key (or stops until a key is set).',
    needKey: 'Enter the {provider} API key first.',
    notConfigured: 'HalloArus is not active yet. Ask the business owner to set an AI API key on this page.',
    notConfiguredManage: 'Set an AI API key (Claude, ChatGPT, Gemini or Qwen) to activate HalloArus.'
  },
  providers: {
    anthropic: { name: 'Claude (Anthropic)', placeholder: 'sk-ant-…', console: 'console.anthropic.com' },
    openai: { name: 'ChatGPT (OpenAI)', placeholder: 'sk-…', console: 'platform.openai.com' },
    gemini: { name: 'Gemini (Google)', placeholder: 'AIza…', console: 'aistudio.google.com' },
    qwen: { name: 'Qwen (Alibaba)', placeholder: 'sk-…', console: 'modelstudio.console.alibabacloud.com (Singapore region)' }
  },
  models: {
    'claude-sonnet-5-5': 'Claude Sonnet 5.5 (balanced, recommended)',
    'claude-opus-5-5': 'Claude Opus 5.5 (deeper analysis)',
    'claude-fable-5-1': 'Claude Fable 5.1 (most capable)',
    'claude-haiku-5-5': 'Claude Haiku 5.5 (fast & cheap)',
    'gpt-5': 'GPT-5 (balanced)',
    'gpt-5-mini': 'GPT-5 mini (fast & cheap)',
    'gpt-5.6-sol': 'GPT-5.6 Sol (newest)',
    'gemini-2.5-pro': 'Gemini 2.5 Pro (deeper analysis)',
    'gemini-2.5-flash': 'Gemini 2.5 Flash (fast & cheap)',
    'gemini-3-flash-preview': 'Gemini 3 Flash (preview)',
    'qwen3-max': 'Qwen3 Max (most capable)',
    'qwen-plus': 'Qwen Plus (balanced)',
    'qwen-flash': 'Qwen Flash (fast & cheap)'
  },
  free: {
    banner: 'Free account: {remaining} of {limit} questions left today. Add your own API key for unlimited use.',
    exhausted: 'The free quota for today is used up (resets tomorrow). Add your own API key to continue without limits.',
    useOwn: 'Use my own key'
  },
  connector: {
    button: 'AI connector',
    title: 'Connect to an AI app',
    intro: 'Use your own Claude, ChatGPT, Gemini or Qwen subscription (no API key): add ARUS as an MCP connector, then ask about your business data right in that app. The connector is read-only, with the same limits as HalloArus.',
    appsTitle: 'Choose your app',
    appsNote: 'Connector support differs per app and plan. If it is unavailable, use the HalloArus chat with an API key (settings above).',
    cmdLabel: 'Command',
    apps: {
      claude: {
        name: 'Claude',
        s1: 'Open Settings → Connectors → Add custom connector.',
        s2: 'Enter a name (e.g. ARUS), paste the connector address above, and save.',
        s3: 'Click Connect; you are sent to ARUS to approve read access.'
      },
      chatgpt: {
        name: 'ChatGPT',
        s1: 'Open Settings → Apps & Connectors → Advanced settings and enable Developer mode (certain plans only).',
        s2: 'Back in Apps & Connectors → Create. Enter a name (e.g. ARUS), paste the connector address above as the MCP Server URL, choose OAuth authentication, then Create.',
        s3: 'In a new chat pick ARUS from the + menu → Developer mode, then approve read access when redirected to ARUS.'
      },
      gemini: {
        name: 'Gemini (Gemini CLI)',
        s1: 'Install the Gemini CLI, then run the command below.',
        s2: 'Run gemini, type /mcp auth arus, then approve read access in the browser that opens.',
        s3: 'Ask about your business data right in the Gemini CLI. The Gemini web app does not support custom connectors yet.'
      },
      qwen: {
        name: 'Qwen (Qwen Code)',
        s1: 'Install Qwen Code, then run the command below.',
        s2: 'Run qwen, finish the authorisation that appears (/mcp) and approve read access in the browser.',
        s3: 'Ask about your business data right in Qwen Code.'
      }
    },
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
