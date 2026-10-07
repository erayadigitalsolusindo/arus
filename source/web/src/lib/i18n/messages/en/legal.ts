import type { Messages } from '../../types.ts';

const legal: Messages['legal'] = {
  draft: 'Draft — this text has not been reviewed by legal counsel and must be reviewed before a public release.',
  updated: 'Effective October 2026',
  back: 'Back',
  terms: {
    docTitle: 'Terms of Service | ACIRABA',
    title: 'Terms of Service',
    s1: {
      title: '1. The service',
      body: 'ACIRABA is a web-based point-of-sale, inventory, and bookkeeping application for retail and food-and-beverage businesses. By registering, you confirm you are authorised to act for the business being registered.'
    },
    s2: {
      title: '2. Accounts and security',
      body: 'You are responsible for keeping your password confidential and for all activity in your business account, including employee accounts you create. Report suspected misuse immediately.'
    },
    s3: {
      title: '3. Business data',
      body: 'The transaction, inventory, and customer data you enter remains yours. We process it only to provide the service, and each business is logically separated from every other business.'
    },
    s4: {
      title: '4. Fair use',
      body: 'You may not attempt to access another business’s data, disrupt the service, or use it for activities that violate applicable Indonesian law.'
    },
    s5: {
      title: '5. Availability and backups',
      body: 'We strive to keep the service available and take regular backups, but we do not guarantee uninterrupted service. You are encouraged to export important reports regularly.'
    },
    s6: {
      title: '6. Limitation of liability',
      body: 'To the extent permitted by law, our liability is limited to the fees you paid for the relevant period, and we are not liable for indirect losses.'
    },
    s7: {
      title: '7. Changes to the terms',
      body: 'We may update these terms. Material changes will be announced in the app or by email, and continued use means you accept the updated terms.'
    },
    s8: {
      title: '8. Governing law',
      body: 'These terms are governed by the laws of the Republic of Indonesia. Disputes will first be resolved amicably before legal action is taken.'
    }
  },
  privacy: {
    docTitle: 'Privacy Policy | ACIRABA',
    title: 'Privacy Policy',
    s1: {
      title: '1. Data we collect',
      body: 'Account data (name, email, phone number), business data you enter (outlets, items, transactions, customers), and technical data such as IP address and access time for security and auditing.'
    },
    s2: {
      title: '2. How we use it',
      body: 'To provide and secure the service, send transactional emails (verification and password reset), and meet legal obligations. We do not sell your data.'
    },
    s3: {
      title: '3. Storage and security',
      body: 'Passwords are stored as one-way hashes. Each business’s data is logically separated in the database, access is limited by role, and important changes are recorded in the audit log.'
    },
    s4: {
      title: '4. Sharing',
      body: 'Data is shared only with infrastructure providers that help run the service (e.g. hosting and email delivery) under confidentiality obligations, or where required by law.'
    },
    s5: {
      title: '5. Your rights',
      body: 'You may request access to, correction of, or deletion of your personal data under applicable Indonesian personal data protection rules by contacting us.'
    },
    s6: {
      title: '6. Retention',
      body: 'Data is kept while your account is active and as long as needed for legal, accounting, or dispute purposes. Afterwards it is deleted or anonymised.'
    },
    s7: {
      title: '7. Contact and changes',
      body: 'Privacy questions can be sent to the service administrator. Policy changes will be announced in the app or by email.'
    }
  }
};

export default legal;
