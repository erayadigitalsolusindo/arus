// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appName => 'ARUS';

  @override
  String get loginTitleLine1 => 'Cashier Access';

  @override
  String get loginTitleLine2 => 'ARUS (Aciraba Upgrade System)';

  @override
  String get loginSubtitle => 'Sign in to continue to your workspace';

  @override
  String get loginEmail => 'Email address';

  @override
  String get loginEmailHint => 'you@company.com';

  @override
  String get loginPassword => 'Password';

  @override
  String get loginShowPassword => 'Show password';

  @override
  String get loginHidePassword => 'Hide password';

  @override
  String get loginRemember => 'Keep me signed in';

  @override
  String get loginSubmit => 'Sign in';

  @override
  String get loginEmailRequired => 'Email is required.';

  @override
  String get loginEmailInvalid => 'Invalid email format.';

  @override
  String get loginPasswordRequired => 'Password is required.';

  @override
  String get loginServer => 'Server';

  @override
  String get logout => 'Sign out';

  @override
  String homeWelcome(String name) {
    return 'Welcome, $name';
  }

  @override
  String get homeOutlet => 'Active outlet';

  @override
  String get homeTenant => 'Business';

  @override
  String get homeComingSoon => 'The cashier screen is being prepared.';

  @override
  String get errorNetwork => 'Cannot reach the server.';

  @override
  String get errorTimeout => 'The server did not respond. Try again.';

  @override
  String get errorUnknown => 'Something went wrong. Try again.';

  @override
  String get errorInvalidCredentials => 'Incorrect email or password.';

  @override
  String errorInvalidCredentialsLeft(int count) {
    return 'Incorrect email or password. $count attempts left before the account is temporarily locked.';
  }

  @override
  String get errorAccountDisabled =>
      'Your account is disabled. Contact your administrator.';

  @override
  String get errorNoOutlet =>
      'Your account has no active outlet. Contact your administrator.';

  @override
  String errorAccountLocked(int minutes) {
    return 'Too many failed attempts. Try again in $minutes minutes.';
  }

  @override
  String get errorRateLimited => 'Too many attempts. Try again in a moment.';

  @override
  String get errorUnavailable =>
      'Service temporarily unavailable. Try again later.';

  @override
  String get errorInternal => 'A server error occurred.';

  @override
  String get errorSessionInvalid =>
      'Your session has ended. Please sign in again.';
}
