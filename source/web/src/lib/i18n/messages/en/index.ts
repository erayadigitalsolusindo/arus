import type { Messages } from '../../types.ts';
import common from './common.ts';
import errors from './errors.ts';
import auth from './auth.ts';
import nav from './nav.ts';
import shell from './shell.ts';
import dashboard from './dashboard.ts';
import iam from './iam.ts';
import account from './account.ts';
import outlets from './outlets.ts';
import audit from './audit.ts';
import legal from './legal.ts';

const en: Messages = { common, errors, auth, nav, shell, dashboard, iam, account, outlets, audit, legal };

export default en;
