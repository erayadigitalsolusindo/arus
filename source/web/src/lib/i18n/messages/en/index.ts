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
import platform from './platform.ts';
import catalog from './catalog.ts';
import items from './items.ts';
import stock from './stock.ts';
import pos from './pos.ts';
import members from './members.ts';

const en: Messages = { common, errors, auth, nav, shell, dashboard, iam, account, outlets, audit, legal, platform, catalog, items, stock, pos, members };

export default en;
