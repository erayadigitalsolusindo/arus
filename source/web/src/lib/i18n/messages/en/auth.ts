import type { Messages } from '../../types.ts';

const auth: Messages['auth'] = {
  login: {
    docTitle: 'Sign In | ACIRABA',
    titleLine1: 'Access Back Office',
    titleLine2: 'ARUS (Aciraba Upgrade System)',
    subtitle: 'Sign in to continue to your workspace',
    orEmail: 'or sign in with email',
    email: 'Email address',
    emailPlaceholder: 'you@company.com',
    password: 'Password',
    forgot: 'Forgot password?',
    showPassword: 'Show password',
    hidePassword: 'Hide password',
    remember: 'Keep me signed in',
    submit: 'Sign In',
    noAccount: "Don't have an account?",
    createOne: 'Create one',
    downloadApk: 'DOWNLOAD CASHIER APK',
    emailRequired: 'Email is required.',
    emailInvalid: 'Invalid email format.',
    passwordRequired: 'Password is required.'
  },
  register: {
    docTitle: 'Sign Up | ACIRABA',
    title: 'Create your ARUS account',
    subtitle: 'Register your business and get started in minutes',
    businessName: 'Business name',
    businessNamePlaceholder: 'Maju Jaya Store',
    ownerName: 'Owner name',
    ownerNamePlaceholder: 'Budi Santoso',
    outletName: 'First outlet name',
    outletNamePlaceholder: 'Main Branch',
    email: 'Email',
    phone: 'Phone number',
    phonePlaceholder: '0812 3456 7890',
    password: 'Password',
    passwordPlaceholder: 'Min. 10 characters, letters and numbers',
    confirmPassword: 'Confirm password',
    mismatch: 'Passwords do not match.',
    strength: { weak: 'Weak', fair: 'Fair', strong: 'Strong' },
    submit: 'Create account',
    haveAccount: 'Already have an account?',
    signIn: 'Sign in',
    headline: 'Start running your business with ARUS.',
    step1: 'Owner account and first outlet are created automatically',
    step2: 'Import products from Excel or CSV',
    step3: 'Enter opening stock, receivables, and payables',
    step4: 'Start selling at the register',
    stepsTitle: 'What happens after you sign up'
  },
  promo: {
    headline: 'Entrepreneurs Create Jobs!',
    point1: 'The most complete ecosystem and integrations to maximize your business opportunities at the next level.',
    point2: 'Our application helps your business soar.',
    point3: 'Point of Sale with a full range of features, easy to use, with accurate data for better business strategy.',
    point4: 'White-label to build your own business brand.',
    point5: 'Aciraba is here to support the growth of small and medium businesses.',
    scanRead: 'Scanned',
    scanTagline: 'Scan & sell in a snap',
    salesToday: "Today's sales",
    transactions: { one: '{count} transaction', other: '{count} transactions' }
  }
};

export default auth;
