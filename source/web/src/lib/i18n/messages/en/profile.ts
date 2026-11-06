import type { Messages } from '../../types.ts';

const profile: Messages['profile'] = {
  docTitle: 'My Profile | ACIRABA',
  title: 'My Profile',
  loadFailed: 'Could not load your profile.',
  retry: 'Try again',
  banner: { change: 'Change banner', add: 'Add banner', remove: 'Remove banner', alt: 'Profile banner' },
  avatar: { change: 'Change photo', add: 'Add photo', remove: 'Remove photo', alt: 'Profile photo of {name}' },
  image: {
    uploading: 'Uploading…',
    tooBig: 'Image must be 10 MB or smaller.',
    badType: 'Choose a JPG, PNG, or WebP image.',
    hint: 'JPG, PNG, or WebP, up to 10 MB.',
    bannerHint: 'Banners look best with a wide image (about 4:1).'
  },
  outlets: { one: '{count} outlet', other: '{count} outlets' },
  verified: 'Email verified',
  unverified: 'Email not verified',
  memberSince: 'Joined {date}',
  lastLogin: 'Last sign-in {date}',
  noLogin: 'Never signed in',
  about: {
    title: 'About',
    empty: 'No bio yet. Tell people a little about your role.',
    addBio: 'Write a bio',
    contact: 'Contact'
  },
  completeness: {
    title: 'Profile completeness',
    done: 'Your profile is complete. Nice work!',
    next: 'Next steps',
    percent: '{value}%',
    step: {
      avatar: 'Add a profile photo',
      phone: 'Add your phone number',
      job_title: 'Add your job title',
      location: 'Add your location',
      bio: 'Write a short bio',
      banner: 'Add a banner',
      email_verified: 'Verify your email',
      mfa: 'Turn on two-step verification'
    }
  },
  tabs: { label: 'Profile sections', overview: 'Overview', info: 'Information', security: 'Security', prefs: 'Preferences' },
  info: {
    title: 'Personal information',
    subtitle: 'This appears on your profile and in activity records.',
    name: 'Full name',
    namePh: 'Your name',
    email: 'Email',
    emailHint: 'Your email is used to sign in and cannot be changed here.',
    phone: 'Phone number',
    phonePh: '0812 3456 7890',
    phoneHint: 'A leading 0 becomes +62 automatically.',
    jobTitle: 'Job title',
    jobTitlePh: 'e.g. Store Manager',
    location: 'Location',
    locationPh: 'e.g. Bandung',
    bio: 'Bio',
    bioPh: 'Tell people a little about yourself or your role…',
    bioHint: 'GitHub-style markdown: **bold**, *italic*, ~~strike~~, `code`, lists, - [ ] tasks, quotes, tables, and links.',
    save: 'Save changes',
    saving: 'Saving…',
    saved: 'Profile saved.',
    discard: 'Discard',
    dirty: 'You have unsaved changes'
  },
  security: {
    title: 'Change password',
    subtitle: 'Use a unique password you do not use anywhere else.',
    current: 'Current password',
    next: 'New password',
    confirm: 'Repeat new password',
    show: 'Show password',
    hide: 'Hide password',
    hint: 'At least 10 characters with letters and numbers.',
    mismatch: 'Passwords do not match.',
    submit: 'Change password',
    saving: 'Saving…',
    logoutNote: 'After changing it, every device is signed out and you will need to sign in again.',
    strength: { label: 'Strength', weak: 'Weak', good: 'Good', strong: 'Strong' },
    accounts: {
      title: 'Accounts & access',
      google: 'Google',
      googleLinked: 'Linked',
      googleNotLinked: 'Not linked',
      googleNote: 'A linked Google account can be used to sign in.',
      pin: 'Approver PIN',
      pinNote: 'The PIN approves sensitive actions at the register.',
      pinManage: 'Manage PIN'
    }
  },
  stats: { actions: 'Activity, 30 days', logins: 'Sign-ins, 30 days', outlets: 'Outlets', tenure: 'Member for', days: '{count} days' },
  overview: {
    access: { title: 'Access & role', role: 'Role', all: 'Full access to every module', modules: '{count} modules available', posOnly: 'Cashier-only account', locked: '{count} modules locked by plan' },
    outlet: { title: 'Active outlet', current: 'This session', total: 'Total access: {count}' },
    activity: {
      title: 'Recent activity',
      subtitle: 'A trail of your own actions in ARUS.',
      empty: 'No activity recorded yet.',
      more: 'Load more',
      loading: 'Loading…',
      failed: 'Could not load activity.',
      from: 'from {ip}',
      actions: {
        user_profile: 'Updated profile',
        user_profile_image: 'Changed profile photo/banner',
        user_password_change: 'Changed password',
        user_mfa_enable: 'Turned on two-step verification',
        user_mfa_disable: 'Turned off two-step verification',
        user_mfa_recovery: 'Regenerated recovery codes',
        user_mfa_reset: "Reset a user's 2FA",
        user_pin_set: 'Set approver PIN'
      }
    }
  },
  mfa: {
    title: 'Two-step verification (2FA)',
    subtitle: 'Add a security layer: besides your password, sign-in asks for a 6-digit code from an authenticator app.',
    on: 'On',
    off: 'Off',
    enable: 'Turn on 2FA',
    disable: 'Turn off 2FA',
    stepScan: '1. Scan this QR code with Google Authenticator, Authy, or 1Password.',
    qrAlt: 'QR code for the authenticator app',
    manual: 'Cannot scan? Enter this key manually:',
    stepCode: '2. Enter the 6-digit code shown in the app.',
    code: '6-digit code',
    verify: 'Verify & turn on',
    cancel: 'Cancel',
    recoveryTitle: 'Save your recovery codes',
    recoveryNote: 'These codes are shown only once. Each can be used one time if you lose your authenticator device.',
    copy: 'Copy all',
    copied: 'Copied',
    download: 'Download .txt',
    saved: 'I have saved them',
    remaining: { one: '{count} recovery code left', other: '{count} recovery codes left' },
    regenerate: 'Regenerate recovery codes',
    regenerateNote: 'Old codes stop working immediately. Enter the 6-digit code from your authenticator app.',
    disableNote: 'Enter your password and a 6-digit code (or one recovery code) to turn off 2FA.',
    password: 'Password',
    recoveryCodeOrTotp: '6-digit code or recovery code',
    confirmDisable: 'Turn off',
    lostDevice: 'Lost your device and recovery codes? Ask an owner/admin to reset your 2FA.',
    loadFailed: 'Could not load 2FA status.',
    done: '2FA is on.'
  },
  prefs: {
    title: 'Preferences',
    subtitle: 'Display settings for this account on this device.',
    language: 'Interface language',
    languageNote: 'The language is saved in this browser and applies to every page.'
  }
};

export default profile;
