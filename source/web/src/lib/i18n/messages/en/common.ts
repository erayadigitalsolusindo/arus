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
  },
  errorPage: {
    notFoundTitle: 'Lost in the deep sea',
    notFoundBody: "The page you're looking for couldn't be found. It may have moved, or the link is wrong.",
    serverTitle: 'The waves are rough',
    serverBody: 'Something went wrong on our server. Please try again in a moment.',
    home: 'Back to home',
    retry: 'Reload'
  }
};

export default common;
