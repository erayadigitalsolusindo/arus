import type { Messages } from '../../types.ts';
import common from './common.ts';
import errors from './errors.ts';
import auth from './auth.ts';
import nav from './nav.ts';
import shell from './shell.ts';
import dashboard from './dashboard.ts';

const en: Messages = { common, errors, auth, nav, shell, dashboard };

export default en;
