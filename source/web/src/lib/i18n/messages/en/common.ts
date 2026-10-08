import type { Messages } from '../../types.ts';

const common: Messages['common'] = {
  appName: 'ACIRABA',
  help: 'Help',
  language: 'Language',
  theme: { light: 'Light mode', dark: 'Dark mode' },
  paid: 'Paid',
  live: 'Live',
  close: 'Close',
  markdown: {
    write: 'Write',
    preview: 'Preview',
    empty: 'Nothing to preview.',
    hint: 'Markdown supported: **bold**, *italic*, lists, links. Raw HTML is not rendered.'
  }
};

export default common;
