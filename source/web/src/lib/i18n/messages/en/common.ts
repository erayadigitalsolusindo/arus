import type { Messages } from '../../types.ts';

const common: Messages['common'] = {
  appName: 'ACIRABA',
  help: 'Help',
  language: 'Language',
  theme: { light: 'Light mode', dark: 'Dark mode' },
  paid: 'Paid',
  live: 'Live',
  close: 'Close',
  dateRange: 'Select date range',
  datePicker: { choose: 'Choose date', clear: 'Clear', today: 'Today', prev: 'Previous month', next: 'Next month' },
  markdown: {
    write: 'Write',
    preview: 'Preview',
    empty: 'Nothing to preview.',
    hint: 'Markdown supported: **bold**, *italic*, lists, links. Raw HTML is not rendered.'
  }
};

export default common;
