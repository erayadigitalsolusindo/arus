import type { Messages } from '../../types.ts';

const errors: Messages['errors'] = {
  NETWORK: 'Unable to connect to the server.',
  UNKNOWN: 'An unexpected error occurred.',
  UNAUTHORIZED: 'Your session has expired or you are not signed in.',
  FORBIDDEN: 'You do not have permission to perform this action.',
  NOT_FOUND: 'The requested data or service was not found.',
  VALIDATION: 'The submitted data is invalid.',
  STOCK_INSUFFICIENT: 'Insufficient stock.',
  INTERNAL: 'A server error occurred.',
  BAD_REQUEST: 'The request could not be processed.',
  UNSUPPORTED_MEDIA_TYPE: 'Unsupported data format.',
  PAYLOAD_TOO_LARGE: 'The submitted data is too large.',
  EMAIL_TAKEN: 'This email is already registered. Please sign in or use another email.',
  INVALID_CREDENTIALS: 'Incorrect email or password.',
  ACCOUNT_DISABLED: 'Your account has been disabled. Contact your administrator.',
  SESSION_INVALID: 'Your session has ended. Please sign in again.',
  TOKEN_EXPIRED: 'Your session has ended. Please sign in again.',
  ACCOUNT_LOCKED: { one: 'Too many failed attempts. Try again in {count} minute.', other: 'Too many failed attempts. Try again in {count} minutes.' },
  RATE_LIMITED: 'Too many attempts. Please try again in a moment.',
  UNAVAILABLE: 'The service is temporarily unavailable. Try again later.',
  FIELD_REQUIRED: 'This field is required.',
  FIELD_INVALID: 'Invalid value or contains disallowed characters.',
  FIELD_TOO_LONG: 'Too long.',
  FIELD_TOO_SHORT: 'Too short.',
  FIELD_WEAK: 'Too weak: use a mix of letters and numbers.'
};

export default errors;
