import type { Messages } from '../../types.ts';

const account: Messages['account'] = {
  verifyBanner: {
    text: 'Your email is not verified yet. Check your inbox for the verification link.',
    resend: 'Resend email',
    sending: 'Sending…',
    sent: 'Verification email sent. Check your inbox (and spam folder).'
  },
  terms: {
    accept: 'I agree to the',
    termsLink: 'Terms of Service',
    and: 'and',
    privacyLink: 'Privacy Policy',
    required: 'You must accept the terms of service and privacy policy.'
  },
  forgot: {
    docTitle: 'Forgot Password | ACIRABA',
    title: 'Forgot your password?',
    subtitle: 'Enter your account email. We will send a link to create a new password.',
    email: 'Email',
    emailPlaceholder: 'you@company.com',
    submit: 'Send reset link',
    sending: 'Sending…',
    sentTitle: 'Check your email',
    sent: 'If that email is registered, a link to reset your password has been sent. The link is valid for 30 minutes and can be used once.',
    back: 'Back to sign in'
  },
  reset: {
    docTitle: 'Reset Password | ACIRABA',
    title: 'Create a new password',
    subtitle: 'Choose a new password for your account. All devices will be signed out.',
    password: 'New password',
    confirm: 'Repeat password',
    mismatch: 'Passwords do not match.',
    submit: 'Save password',
    saving: 'Saving…',
    successTitle: 'Password changed',
    success: 'Please sign in with your new password.',
    missing: 'The link is incomplete. Open the link from your email, or request a new one.',
    requestNew: 'Request a new link',
    login: 'Go to sign in'
  },
  verify: {
    docTitle: 'Verify Email | ACIRABA',
    working: 'Verifying your email…',
    successTitle: 'Email verified',
    success: 'Thank you. Your email address is now verified.',
    failedTitle: 'Invalid link',
    failed: 'The verification link is invalid or has expired. Sign in and request a new verification email from the banner at the top.',
    toDashboard: 'Go to dashboard',
    toLogin: 'Go to sign in'
  }
};

export default account;
