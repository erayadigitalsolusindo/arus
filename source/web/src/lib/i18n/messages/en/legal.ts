import type { Messages } from '../../types.ts';

const legal: Messages['legal'] = {
  updated: 'Effective October 2026',
  back: 'Back',
  nav: 'Legal documents',
  search: {
    placeholder: 'search this policy, e.g. cookie, camera, delete…',
    placeholderTerms: 'search these terms, e.g. payment, illegal, delete…',
    placeholderHelp: 'search help, e.g. shift, printer, stock, QRIS…',
    hint: 'Press / to search',
    count: '{count} matches in {sections} sections',
    none: '0 matches',
    empty: 'No results for "{q}". Try another word, e.g. "data", "cookie", or "delete".',
    clear: 'Clear search'
  },
  terms: {
    docTitle: 'Terms of Service | ARUS',
    title: 'Terms of Service',
    s1: {
      title: '1. Short summary',
      body: `- ARUS is a point-of-sale, inventory, purchasing, reporting, and accounting service for retail and food-and-beverage businesses. By registering or using it, you agree to these terms.
- Your business data stays yours. We only process it to provide the service (see the Privacy Policy).
- ARUS may only be used for lawful transactions that comply with the laws of Indonesia and of the country where your business operates. Illegal transactions are strictly prohibited (section 7).
- We may suspend or terminate accounts that violate these terms and cooperate with competent authorities.
- You are responsible for your own account, staff, transactions, bookkeeping, and taxes.`
    },
    s2: {
      title: '2. Acceptance and definitions',
      body: `These terms are an agreement between you and Eraya Digital Solusindo ("we"). By creating an account, signing in, or using ARUS, you confirm that you have read and agree to them. If you register on behalf of a business, you confirm you have authority to bind it.

| Term | Meaning
| Service | ARUS Web, ARUS Mobile (Android), the external API, the developer portal, and the print-agent, including their updates
| Tenant or business | One business account with all of its outlets, users, and data
| User | Anyone given access to a tenant, including owners, admins, cashiers, and API service accounts
| Your Data | Business, customer, transaction, stock, and bookkeeping data you enter into the service
| Platform Admin | Our staff who manage the service as a whole`
    },
    s3: {
      title: '3. The service we provide',
      body: `- Point of sale (web and Android), shifts, petty cash, returns, credit and receivables, members, and receipt printing.
- Stock and master data: stock ledger, stock count, inter-branch transfers, purchasing, payables, and per-branch pricing.
- Reports, dashboards, and accounting (SIAK), with Excel and PDF export.
- Optional features: the HalloArus AI assistant, the external API, QRIS payments through EraPay, and custom domains.
- Subscription plans with different limits and modules (section 5).

The service keeps evolving. We may add, change, or discontinue features; for major changes we announce them in advance in the app or by email. Some features are still under development and may change.`
    },
    s4: {
      title: '4. Accounts and security',
      body: `- You must be at least 18 years old and legally capable to create an account.
- Registration information must be accurate and you must keep it up to date. One tenant represents one business.
- The tenant owner is responsible for all activity in their tenant, including staff accounts they create, roles and permissions they grant, and approver PINs.
- Keep your password, PIN, 2FA codes, and API credentials confidential. Do not share accounts; create a separate user for each person.
- Deactivate accounts of staff who no longer work for you.
- Contact us immediately if you suspect your account has been misused.
- We are not liable for losses caused by your failure to protect your credentials.`
    },
    s5: {
      title: '5. Subscription plans and payment',
      body: `- The service is offered in several plans that differ in daily transaction limits, number of users, and accessible modules. Current details and prices are shown on the Subscription page.
- Exceeding a plan limit causes new transactions or new users to be rejected until you upgrade or the limit becomes available again. Modules outside your plan appear locked.
- Plan payments are made by QRIS or other available methods. Validity and renewal follow the information on the Subscription page.
- Prices and plan contents may change. Changes do not affect periods you have already paid for.
- Fees already paid are non-refundable, unless required by law or agreed in writing by us.
- You are responsible for third-party costs arising from your use, such as payment provider MDR, AI API costs, and internet fees.`
    },
    s6: {
      title: '6. Acceptable and prohibited use',
      body: `You may use ARUS to run your own business. You must not:

- try to access another tenant's data, bypass plan limits, or break security controls, permissions, or Row Level Security;
- disrupt or unreasonably burden the service, including load testing or scanning without written permission;
- use bots, scripts, or the API outside what is permitted, or share API credentials;
- copy, decompile, reverse engineer, resell, lease, or rebrand the service without our written permission;
- upload malware, content that infringes others' rights, or unlawful content;
- enter other people's personal data without a valid legal basis;
- use the service for any activity that conflicts with section 7.`
    },
    s7: {
      title: '7. Transactions must be lawful',
      body: `ARUS may only be used to record and process lawful transactions. You represent and warrant that all transactions, goods, services, and bookkeeping you process through ARUS comply with the laws of the Republic of Indonesia and of the country where your business operates, including business licensing, taxation, consumer protection, and personal data protection. Where local law differs, the stricter rule applies.

It is strictly prohibited to use ARUS for:

| Prohibited | Examples
| Illegal goods or services | Narcotics and prohibited substances, illegal weapons or explosives, stolen or counterfeit goods, protected wildlife and plants, and goods banned from trade
| Gambling and fraud | Illegal gambling, pyramid or Ponzi schemes, sales fraud, and falsified transactions
| Money laundering and terrorist financing | Disguising the source of funds, splitting transactions to avoid reporting, or funding prohibited activity (Indonesian Law No. 8 of 2010 and Law No. 9 of 2013)
| Falsified books and tax evasion | Creating fictitious receipts or journals, hiding sales, or manipulating reports to avoid tax obligations
| Violating consumer rights | Selling expired or dangerous goods, misleading prices, or withholding consumer rights (Law No. 8 of 1999)
| Violating personal data and electronic systems rules | Using customer data without a lawful basis, illegal access, or misuse of electronic systems (Law No. 27 of 2022 and the ITE Law)
| Violating sanctions and licensing | Dealing with sanctioned parties or selling licensed goods without the required licence

Consequences of a violation:

- We do not monitor the content of every transaction, and the lawfulness of your transactions is entirely your responsibility.
- If we learn of or reasonably suspect a violation, we may immediately restrict, suspend, or terminate the account without prior notice and withhold related services.
- We may preserve evidence (including audit logs) and disclose it to competent authorities, and cooperate with law enforcement, regulators, and payment providers as required by regulations.
- A violation does not remove your payment obligations and does not limit our right to take legal action.
- You bear all legal consequences of the transactions you carry out, and release us from third-party or authority claims arising from them (section 16).

If you see ARUS being misused, report it to info@erayadigital.co.id.`
    },
    s8: {
      title: '8. Your data and privacy',
      body: `- Your Data remains yours. You give us a limited licence to store, process, and back it up solely to provide the service.
- For personal data of customers and staff you enter, you are the data controller and we are the data processor on your behalf. You must have a valid legal basis and inform the people whose data you process.
- Each tenant's data is separated on the server and in the database. How we process and protect data is described in the Privacy Policy (/privacy), which forms part of these terms.
- You can export reports at any time (Excel, PDF, and NDJSON for the audit log). We recommend regular exports as your own backup.`
    },
    s9: {
      title: '9. Third-party payments (QRIS and EraPay)',
      body: `- Non-cash payments such as QRIS are processed by third-party payment providers (for example EraPay). We are not a bank, financial institution, or payment operator, and we do not hold your funds.
- Use of EraPay is subject to that provider's terms, including merchant verification (KYC), MDR fees, and settlement schedules.
- You are responsible for making sure payment-method, MDR, and amount settings in ARUS match your agreement with the payment provider.
- Payment status depends on the provider. The "force save" feature is only for when the gateway is unreachable, requires an approver PIN, and flags the receipt for re-checking. You must reconcile it with the provider.
- Disputes, refunds, and chargebacks with customers or providers are between you and those parties.`
    },
    s10: {
      title: '10. AI features, reports, accounting, and tax',
      body: `- Answers from the AI assistant (HalloArus or the MCP connector) are generated automatically and may be wrong or incomplete. Verify them before relying on them for decisions.
- AI output, dashboards, reports, and automatic journals are aids. None of them is legal, tax, financial, or accounting advice.
- You and your accountant are responsible for the accuracy of your books, financial statements, and tax obligations, including issuing any required invoices or tax documents.
- Receipts and invoices from ARUS are your business's transaction records; we do not issue them in our name.`
    },
    s11: {
      title: '11. Intellectual property and licence',
      body: `- The service, code, design, ARUS brand, and documentation belong to us or our licensors. Use of the source code is governed by the ARUS Source-Available License in the LICENSE file: the code may be read and used for your own business, but resale without permission is prohibited.
- We grant you a limited, non-exclusive, non-transferable, revocable right to use the service while these terms apply.
- You keep ownership of your data and content (business name, logo, item images). You warrant that you have the right to use them and that they do not infringe others' rights.
- We may use your feedback or suggestions to improve the service without any obligation to you.
- White-label or custom-domain features may only be used as set up by Platform Admin, and give you no ownership of the service.`
    },
    s12: {
      title: '12. External API and integrations',
      body: `- API credentials (key and secret) are confidential and shown only once. You are responsible for storing them securely and rotating them if they leak.
- API access is restricted by IP allowlist, HMAC signature, nonce, and rate limits. Circumventing these limits is prohibited.
- Every API request runs as a service account with its role's permissions; you are responsible for the permissions you grant and for all activity performed with your credentials.
- We may change, restrict, or revoke API access, including in case of abuse or security threats.
- Integrations you build with third parties are your responsibility.`
    },
    s13: {
      title: '13. Availability, maintenance, and backups',
      body: `- We work to keep the service available but do not guarantee it is free from interruption or errors, or always accessible. There is no service level agreement (SLA) unless agreed in writing.
- Maintenance and updates may cause temporary downtime. We try to do them at quiet times and notify you when the impact is significant.
- We back up the database regularly and before system updates. Backups exist to restore the service, not as your personal archive.
- The POS apps do not yet support offline mode. Prepare a fallback procedure (for example manual records) for when the connection drops.
- Printers, networks, devices, and power at your premises are outside our control.`
    },
    s14: {
      title: '14. Termination and account deletion',
      body: `- You may stop using the service at any time. Contact us if you want to deactivate or delete a tenant.
- We may suspend or terminate an account if you breach these terms (especially sections 6 and 7), fail to pay due fees, endanger the security of the service, or are required to by law.
- A deactivated tenant can be scheduled for permanent deletion with a waiting period of 0 to 365 days. Afterwards all tenant data is permanently deleted and cannot be restored from the app. Details are in the Privacy Policy.
- Export the data you need before termination, including for bookkeeping and tax obligations.
- Provisions that by their nature survive termination (for example sections 7, 11, 15, 16, and 18) remain binding.`
    },
    s15: {
      title: '15. Warranties and limitation of liability',
      body: `- The service is provided "as is" and "as available". To the extent permitted by law, we give no express or implied warranties, including fitness for a particular purpose and freedom from errors.
- To the extent permitted by law, our total liability to you is limited to the fees you paid for the service in the 12 months before the event giving rise to the claim.
- We are not liable for indirect losses, lost profits, lost business opportunities, or data loss arising from use of or inability to use the service, your negligence, or actions of third parties (including payment, hosting, and AI providers).
- We are not responsible for the lawfulness of your transactions, your bookkeeping, or your tax compliance.
- Nothing here limits liability that cannot be limited by law.`
    },
    s16: {
      title: '16. Indemnity and force majeure',
      body: `You agree to indemnify and hold harmless us, our affiliates, and our staff from claims, fines, losses, and costs (including reasonable legal fees) arising from: your breach of these terms, illegal transactions or activity carried out through your tenant, infringement of others' rights, or violation of applicable law.

We are not liable for delay or failure caused by circumstances beyond reasonable control, such as natural disasters, war, riots, power or network outages, infrastructure provider failures, large-scale cyberattacks, or changes in regulations or orders of authorities.`
    },
    s17: {
      title: '17. Changes to these terms',
      body: 'We may update these terms. Important changes are announced in the app or by email, and the effective date at the top of the page is updated. Continuing to use the service after the effective date means you accept the updated terms. If you do not agree, stop using the service.'
    },
    s18: {
      title: '18. Governing law, disputes, and contact',
      body: `- These terms are governed by the laws of the Republic of Indonesia. For businesses operating in other countries, mandatory local law still applies to your business activities there.
- Disputes are first resolved amicably. If that fails, they are resolved through the competent courts in Indonesia.
- These terms together with the Privacy Policy are the entire agreement between us regarding the service. If any provision is unenforceable, the others remain in effect.
- We may assign our rights and obligations in a business reorganisation, while continuing to protect your data. You may not assign your account without our written consent.
- If a translation differs from the Indonesian version, the Indonesian version prevails.

Questions about these terms: Eraya Digital Solusindo, info@erayadigital.co.id.`
    }
  },
  privacy: {
    docTitle: 'Privacy Policy | ARUS',
    title: 'Privacy Policy',
    s1: {
      title: '1. Short summary',
      body: `- We only process the data needed to run ARUS (point of sale, inventory, purchasing, reports, accounting) for your business.
- We do not sell data, show ads, or use third-party trackers or analytics in the web or Android apps.
- Each business (tenant) is separated on the server and in the database (Row Level Security), so one business cannot see another.
- Passwords are stored as argon2id hashes, never in their original form.
- The AI feature (HalloArus) is optional, and customer data is never sent to AI providers.
- You can request access, correction, or deletion of your data at info@erayadigital.co.id.`
    },
    s2: {
      title: '2. Who we are and our role',
      body: `ARUS is developed and operated by Eraya Digital Solusindo ("we"). Our role differs by type of data.

| Type of data | Our role | What it means
| Your account and business data (registration, users, subscription) | Data controller | We decide the purposes and means of processing.
| Customer, transaction, and stock data you enter | Data processor on behalf of your business | Your business decides; we process it only to provide the service, on your instructions.

If your business stores personal data of customers or employees, your business is responsible for informing them and for having a valid legal basis to process it.`
    },
    s3: {
      title: '3. Data we collect',
      body: `| Category | Examples | Source
| Account and registration | Business name, owner name, email, phone number, outlet name, password (argon2id hash), role and permissions | You, at registration
| Staff (users) | Name, email, role, accessible outlets, approver PIN (stored encoded), active status | Business owner or admin
| Security and trail | IP address, time, action performed, request ID, failed login attempts | Automatically by the server
| Technical | Client type (web or mobile), language, outlet time zone | Automatically
| Subscription and payment | Plan, status, billing history, QRIS transaction ID (not card data) | System and payment provider
| Support | Your message and email address | You, when you contact us`
    },
    s4: {
      title: '4. Customer and transaction data in your business',
      body: `Data you enter into ARUS belongs to your business. We do not use it for any purpose other than providing the service.

| Module | Example data
| Sales and POS | Receipts, line items, prices, payments per method, cashier, shift, petty cash
| Members | Name, gender, phone, email, address, points, deposit, level
| Receivables and payables | Party, amount, installments, due date, settlement
| Stock and purchasing | Items, images, suppliers, purchases, stock count, transfers
| Accounting (SIAK) | Chart of accounts, journals, bank reconciliation

ARUS is not designed to store national ID numbers, biometric data, or payment card numbers. Please do not enter them in note fields.`
    },
    s5: {
      title: '5. Purposes and legal basis',
      body: `- Providing ARUS features: POS, inventory, purchasing, reports, and accounting.
- Authenticating users and preventing abuse (login attempt limits, rate limiting, 2FA for platform admins).
- Recording an audit trail of important changes: who did what, when, and from which IP address.
- Sending transactional email such as email verification and password reset.
- Managing subscriptions and plan payments.
- Maintaining and improving the service, for example from technical error logs.
- Meeting legal, bookkeeping, and dispute-resolution obligations.

Our legal bases are performance of the service agreement with you, your consent, our legitimate interest in keeping the service secure, and legal obligations. We do not sell data and do not build advertising profiles.`
    },
    s6: {
      title: '6. Cookies and browser storage',
      body: `The ARUS web app uses a single essential cookie and a little local storage. There are no advertising or analytics cookies, so there is no cookie consent banner.

| Name or function | Type | Content and lifetime
| Session refresh cookie | Essential cookie: HttpOnly, Secure, SameSite=Lax, path /auth | Session refresh token. Valid for 12 hours, or 30 days if you choose to stay signed in. Not readable by JavaScript.
| Access token | Tab memory | Lives 15 minutes and disappears when the tab is closed.
| Display preferences | Browser local storage | Light or dark theme, POS layout choice (modern or classic), and language.
| Form drafts and filters | Browser storage | Unsaved input and page filters so they are not lost when switching tabs. They exist only on your device.

You can clear all of it in your browser settings; you will simply need to sign in again.`
    },
    s7: {
      title: '7. Mobile app (ARUS Mobile)',
      body: `| Android permission | Purpose | If denied
| Camera | Scan item barcodes. Camera images are never recorded, stored, or sent. | Camera scanning is unavailable; type the code or use an HID scanner.
| Nearby devices (Bluetooth) | Connect to a receipt printer you have already paired in Android settings. | Receipts cannot be printed over Bluetooth.
| Internet | Talk to the ARUS server over HTTPS. | The app cannot be used (there is no offline mode yet).

Stored on the device:

- The session refresh token in Android secure storage (Keystore); removed when you sign out.
- Display preferences, language, and printer settings.
- The saved cart and held receipts per user and branch (items and quantities only; totals are always recalculated by the server).
- All local data is removed when the app is uninstalled.

QRIS payments are processed by the payment provider you connect; we do not store card data. We use no advertising or analytics SDKs and do not track you across apps. The directly downloaded APK checks the latest version number with the ARUS server and downloads updates from the same server; the Google Play version is updated through Google Play.`
    },
    s8: {
      title: '8. Data sharing and third parties',
      body: `Data is shared only as needed, under confidentiality obligations, or where required by law. Data is not sold.

| Recipient | Data | Purpose
| Hosting, server, and network providers | Encrypted traffic and stored data | Running the service
| Email provider (SMTP) | Email address and the content of transactional email (verification or reset links) | Sending email
| EraPay (QRIS) | Amount, transaction reference, and merchant data for activation; no card data | QRIS payments and plan upgrades
| AI provider of your choice (optional) | Anonymised summary figures | The HalloArus feature (see section 9)
| Competent authorities | As officially ordered | Legal obligations

ARUS Platform Admins can open a business account for support through the "sign in as" feature. This access is limited to authorised admins with 2FA and is recorded in the platform audit log.`
    },
    s9: {
      title: '9. AI features (HalloArus and the MCP connector)',
      body: `- The feature is optional and only available to users who have been granted permission.
- Only summary figures (for example revenue, stock, and trends) are sent to the AI provider. Customers appear as "Customer 1, 2, …"; names, phone numbers, and addresses are never sent.
- You choose the provider (Claude, ChatGPT, Gemini, or Qwen) and use your business's own API key or the free ARUS account. The chosen provider's privacy policy applies to the data it receives.
- The MCP connector lets an assistant from your own subscription read ARUS data with the same permissions as your account, read-only. Access tokens last 1 hour and refresh tokens 30 days.`
    },
    s10: {
      title: '10. Security',
      body: `- Connections use HTTPS; production requires https addresses.
- Passwords are hashed with argon2id. Access tokens live 15 minutes, refresh tokens rotate on every use, and login is locked progressively after failed attempts.
- Each business's data is separated with PostgreSQL Row Level Security. Tenant, outlet, and user always come from the server-side token, not from client input.
- Permissions per role and per outlet, an approver PIN for sensitive actions, and mandatory 2FA (TOTP) for Platform Admins.
- The audit log is append-only and cannot be altered.
- The database is backed up automatically before every system update.

No system is completely immune. Keep your password and PIN confidential and report suspected misuse immediately. If a personal data breach occurs, we notify affected parties and the authorities within the time limits set by personal data protection regulations.`
    },
    s11: {
      title: '11. Retention and deletion',
      body: `| Data | Kept for | Afterwards
| Account and business data | While the account and subscription are active | Deleted when the tenant is deleted
| Login sessions | Refresh 12 hours or 30 days; access 15 minutes | Expire automatically, or revoked on sign-out
| Business audit log | While the tenant is active (cannot be altered) | Deleted together with the tenant
| Platform audit log | Retained for accountability | Remains stored
| Database backups | About 30 days in the standard configuration | Deleted automatically
| Deactivated tenant | An admin schedules deletion with a waiting period of 0 to 365 days | All tenant data is permanently deleted, including images

Permanent deletion cannot be undone from the app, and copies may remain in backups until they are overwritten. Before deleting a business, export the reports you need for bookkeeping and tax obligations.`
    },
    s12: {
      title: '12. Your rights',
      body: `Under Indonesian Law No. 27 of 2022 on Personal Data Protection, you have the right to:

- access and obtain a copy of your personal data;
- correct data that is wrong or incomplete;
- request deletion or destruction of data;
- withdraw consent, and restrict or object to certain processing;
- receive your data in a reusable format (reports can be exported to Excel or PDF, and the audit log to NDJSON);
- lodge an objection or complaint with us and with the competent authority.

Send your request to info@erayadigital.co.id from your account's email address. We verify your identity first and reply within a reasonable time in line with regulations. For a store's customer data, contact that business owner as the data controller; we will help them fulfil your request.`
    },
    s13: {
      title: '13. Children',
      body: 'ARUS is intended for business operators and is not meant for children under 18. If we learn that a child\'s data was registered without valid authority, we will delete it.'
    },
    s14: {
      title: '14. Data location and cross-border transfers',
      body: 'Data is stored on servers chosen by the service operator. If a third-party provider you use (for example email or AI) processes data outside Indonesia, the transfer follows applicable personal data protection rules and is covered by contractual obligations.'
    },
    s15: {
      title: '15. Policy changes and contact',
      body: `Important changes to this policy are announced in the app or by email, and the effective date at the top of the page is updated. Continued use means you accept the updated policy.

Privacy questions or requests: Eraya Digital Solusindo, info@erayadigital.co.id.`
    }
  },
  help: {
    docTitle: 'Help Center | ARUS',
    title: 'Help Center',
    s1: {
      title: '1. Quick start',
      body: `The shortest path from sign-up to your first receipt:

- Sign up on the Register page: enter business name, owner name, email, phone number, outlet name, and a password. You automatically become the Owner.
- Verify your email using the link sent to you.
- Prepare basic data under Master Data: units, categories, and payment methods. Cash is built in.
- Add items under Item List (code, barcode, price, unit). Bulk import from Excel or CSV is not available yet.
- Enter opening stock under Opening Stock. It is recorded as an opening balance and can be done only once; afterwards use Stock Count.
- Open the POS screen (/kasir), open a shift with an opening float, and start selling.
- Add cashiers under Users with the right role, and set the approver PIN under PIN.`
    },
    s2: {
      title: '2. Menu map',
      body: `| Menu | What it is for
| Dashboard | "Is the store doing fine?" summary: revenue, trends, best sellers, low stock, branch comparison
| Live Sales | Today's sales, updated in real time
| Item List and Stock Card | Item master and the in and out history of stock with a running balance
| People | Members, suppliers, and salespeople
| Supporting Data | Units, categories, member levels, payment methods, brands, principals
| Sales | Sales list, sales returns, and shopping coupons
| Purchasing and payables | Purchase invoices, purchase returns, and supplier payments
| Reports | Sales, purchasing, returns, payables, receivables, and inventory, exportable to Excel and PDF
| SIAK | Accounting: chart of accounts, journals, ledger, balance sheet, income statement, cash and bank
| HalloArus | A read-only AI assistant to ask about store figures
| Users, Roles and Permissions, Outlets | Manage people, permissions, and branches
| Audit Log | A trail of who changed what and when
| Subscription | Plan, usage limits, and upgrades

A menu that does not appear usually means your role has not been granted permission, or the module is not in your plan (shown with a lock).`
    },
    s3: {
      title: '3. Signing in and accounts',
      body: `- Sign in with email and password. Tick "Keep me signed in" for a 30-day session; otherwise the session lasts 12 hours.
- Forgot your password: use the "Forgot password?" link on the sign-in page. The reset link is sent to your account email.
- Email verification: check your spam folder if nothing arrives. Some features, such as enabling EraPay, require a verified email.
- An account is locked temporarily after repeated wrong passwords (default 5 attempts; locks of 1, 5, 15, then 60 minutes). Wait for the time shown and try again.
- Staff are added by the Owner or an admin under Users. Deactivate accounts of staff who have left.
- Do not share accounts. Create one user per person so the audit log stays accurate.`
    },
    s4: {
      title: '4. Web POS',
      body: `Basic flow: search or scan an item, review the cart, press Pay, choose a payment method, then print the receipt.

- Two layouts: /kasir (modern) and /kasirb (classic desktop style). The logic is the same; your choice is remembered.
- Scan barcodes with a USB scanner (an exact code goes straight into the cart), or type a name or code in the search box.
- Payment: cash, credit (down payment optional), non-cash, and split (mix several methods). Prices, tax, and totals are calculated by the server.
- A receipt can be held as pending and reopened later.
- Members and points, salesperson, coupons, and other charges can be added to the receipt.

Keyboard shortcuts (press F1 in the POS to see the list):

| Key | Function
| F1 | Show the shortcut list
| F2 | Search or scan an item
| F3 | Enter QTY before scanning
| F4 | Choose a member
| F6 | Choose a salesperson
| F7 | Hold as pending receipt
| F8 | Pending receipts list
| F9 | Today's sales
| F10 | Pay
| Esc | Clear search or close a dialog

In the payment dialog: F1 cash, F2 credit, F3 non-cash, F4 split.`
    },
    s5: {
      title: '5. Shifts and petty cash',
      body: `- Receipts can only be saved while a shift is open. If you see "Open a cashier shift first", open one with an opening float.
- When closing a shift, count the physical money per payment method. If there is a difference, a note is required and an approver PIN is needed.
- The shift-close receipt and daily recap can be printed for handover.
- Petty cash (cash in and out) is recorded per shift; amounts above a limit need approval with a PIN.
- Ideally one cashier uses one shift at a time so the drawer is easy to reconcile. The "By Cashier" report shows the difference for each shift.`
    },
    s6: {
      title: '6. Prices, approver PIN, and permissions',
      body: `- Prices are calculated by the server from the price master. Cashiers cannot change prices freely.
- Changing a price or giving a discount on a line needs an Owner or Supervisor as approver, with their PIN. PIN use is recorded in the audit log.
- Set your PIN under PIN. Too many wrong PINs lock the PIN temporarily.
- Permissions are set per role under Roles and Permissions (view, create, update, delete, approve) and can be limited per outlet.
- Cancelling or changing a receipt is done through a revision, so the original receipt's history is preserved.`
    },
    s7: {
      title: '7. Printing receipts',
      body: `| Method | When to use | Requirement
| Direct to printer (ESC/POS) | Recommended: sharp text, fast, automatic paper cut | print-agent installed on the POS PC
| Through the browser | Fallback when the agent is missing | Chrome with the --kiosk-printing option
| Bluetooth (Android) | ARUS Mobile app | A Bluetooth thermal printer already paired in Android settings

Installing the print-agent on the POS PC:

- Install the printer driver, make it the default printer, and run "Print Test Page".
- Extract arus-print-agent-windows.zip and double-click pasang.cmd. No window opens, which is normal.
- Double-click uji-cetak.cmd; the printer should print a test page. If not, check print-agent.log in %LOCALAPPDATA%\\ArusPrintAgent.
- In the app, open Receipt settings and press Test printer. Choose the Automatic print method to fall back to the browser if the agent is off.

The full guide is in PC-KASIR.md in the repository.`
    },
    s8: {
      title: '8. Stock',
      body: `- Stock is a ledger: every change is recorded as a movement and the balance can always be traced in the Stock Card.
- Each branch has three buckets: Display, Warehouse, and Returns.
- Opening stock: enter it once under Opening Stock; after it is locked, corrections are made with a Stock Count.
- Stock Count: compare the physical count with the system (per session or direct). Differences are recorded as adjustments.
- Stock Transfer: move items between branches or buckets (send, then receive). Receiving differences are recorded.
- Unit Conversion: break a large unit into a small one, for example a carton into pieces.
- "Insufficient stock" means the bucket balance is too low. Add stock through purchasing, a transfer, or a count.
- Purchase returns take items from the Returns bucket, so transfer the items to that bucket first.`
    },
    s9: {
      title: '9. Purchasing, payables, and returns',
      body: `- Purchase invoices can be cash or credit, with tiered line discounts and other charges. Cost is the weighted average per branch.
- Supplier payables can be paid in installments or settled in bulk, with payables aging in reports.
- Sales returns are created from the original receipt; stock and receivables adjust accordingly. A return cannot be cancelled once the receipt's payable or receivable has been paid.
- Changing or cancelling an invoice is done through a revision.
- Supplier credit (refunds from returns) can be used to pay payables.`
    },
    s10: {
      title: '10. Members, receivables, and coupons',
      body: `- Members have levels based on points, points can be redeemed at payment, and deposits can be used as a payment method.
- Credit sales create receivables. Receivables can be paid in installments or settled in bulk, and the credit limit is set per member; exceeding it needs an approver.
- Shopping coupons are created under Coupons; a code that has been used cannot be changed.
- Receivables reports show outstanding receivables, payments, balances per member, and a sub-ledger.`
    },
    s11: {
      title: '11. QRIS payments (EraPay)',
      body: `- The owner enables EraPay in the account menu under Ecosystem. The Owner's email must be verified, and a new merchant waits for EraPay review.
- Set the EraPay API key on the web (per tenant). The system re-checks the key when it is saved.
- In the POS choose the QRIS method and press Create QR. The customer scans; the screen checks the status every few seconds and the receipt is saved automatically once it is paid.
- If EraPay cannot be reached, "Force save" is available and needs an approver PIN. The receipt is flagged for re-checking and the system reconciles it automatically.
- Payment method MDR fees are set under Payment Methods, including the small-transaction rule.`
    },
    s12: {
      title: '12. Reports, dashboard, and accounting',
      body: `- Sales reports: by invoice, invoice detail, item, cashier, customer revenue, customer invoices, and by date.
- There are also Purchasing, Returns, Payables, Receivables, Master, and Inventory valuation reports.
- All reports can be exported to Excel and PDF. The date range is at most 366 days; use the outlet, cashier, or category filters to narrow down.
- The dashboard shows a summary and alerts, for example low stock.
- SIAK: journals are created as drafts and then posted by an accountant (they cannot be changed after posting). Sales can be journaled automatically. A ledger, trial balance, income statement, balance sheet, cash and bank, bank reconciliation, and year-end close are available.
- HalloArus answers questions about store figures. Customer data is not sent to the AI provider.`
    },
    s13: {
      title: '13. Android app (ARUS Mobile)',
      body: `- Download the APK from the web sign-in page, or install from Google Play when available. Sign in with the same account as the web.
- The server dialog on the sign-in page lets you choose Production or another address.
- Allow Camera to scan barcodes, and Nearby devices for a Bluetooth printer.
- Pair the thermal printer in Android settings first, then pick it in the app's Printer settings (58 or 80 mm).
- The directly downloaded APK checks for new versions and offers an automatic update. The Google Play version is updated through Google Play.
- Features: POS, shifts, petty cash, members, QRIS, credit, pending receipts, and Today's sales.
- Not available yet: sales returns, offline mode, and BLE-only printers.`
    },
    s14: {
      title: '14. Subscription and plan limits',
      body: `- A plan sets the daily transaction limit, the number of users, and the modules you can open. See details under Subscription.
- If you see "Your plan's daily transaction limit has been reached" or "User limit reached", upgrade the plan or deactivate another user.
- A locked menu means that module is not in your plan; click it to see how to upgrade.
- Upgrades are paid by EraPay QRIS and the plan activates automatically once payment is confirmed.`
    },
    s15: {
      title: '15. Troubleshooting',
      body: `| Symptom | Common cause | What you can do
| "Wrong email or password" | A typo, or the wrong email | Check upper and lower case, or use Forgot password
| "Too many failed attempts" | The account is temporarily locked | Wait for the time shown, then try again
| "Open a cashier shift first" | No shift is open | Open a shift with an opening float
| "Insufficient stock" | The bucket balance is too low | Add stock through purchasing, a transfer, or a count
| Asked for an approver PIN | Changing a price or exceeding a limit | Ask an Owner or Supervisor to enter their PIN
| "PIN locked" | Too many wrong PINs | Wait for the countdown, then try again
| "Plan limit reached" | Daily transactions or users are full | Upgrade the plan under Subscription
| A menu is missing | Role without permission, or a module outside the plan | Ask the Owner to adjust Roles and Permissions or upgrade
| Receipt does not print | The print-agent is off, or the printer is not the default | Run uji-cetak.cmd, see section 7
| "Server unreachable" | The connection is down or the server has a problem | Check your internet; the server status dot in the footer shows its state
| Sign-in loops on your own domain | Domain or SSL configuration is not right | Contact your server administrator`
    },
    s16: {
      title: '16. Frequently asked questions',
      body: `- Can other stores see my store's data? No. Each business's data is separated on the server and in the database. See the Privacy Policy for details.
- Does it support multiple branches? Yes. One tenant can have many outlets with per-branch prices, stock, and permissions.
- Can it be used without internet? Not yet. The web and Android POS have no offline mode.
- Can data from the old application be moved over? There is no automatic migration. Stores start through onboarding: items, opening stock, and opening receivables or payables.
- How do I export data? All reports can be exported to Excel and PDF, and the audit log to NDJSON.
- How do I delete my business? Contact us. The tenant is deactivated first and then permanently deleted after the chosen waiting period.
- Which languages are available? Indonesian, English, and Japanese. Switch with the language icon in the corner of the page.
- May I use it for any kind of transaction? Only lawful ones. See Terms of Service section 7.`
    },
    s17: {
      title: '17. Contact us',
      body: `Need more help? Email info@erayadigital.co.id. To get a faster answer, include:

- your business name and account email (never send your password or PIN);
- the page or menu where the problem occurs and the steps you took;
- a screenshot, plus any error code or message shown;
- the device and version you use (web and browser, or ARUS Mobile).

Developers who want to integrate through the API can read the documentation at /developers. The rules of use are in the Terms of Service (/terms) and data protection in the Privacy Policy (/privacy).`
    }
  }
};

export default legal;
