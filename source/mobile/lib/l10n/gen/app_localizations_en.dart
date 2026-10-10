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

  @override
  String get themeSystem => 'System';

  @override
  String get themeLight => 'Light';

  @override
  String get themeDark => 'Dark';

  @override
  String themeTooltip(String mode) {
    return 'Theme: $mode';
  }

  @override
  String get posTitle => 'Cashier';

  @override
  String get posSearchHint => 'Search name, code or barcode';

  @override
  String get posSearchEmpty => 'No items found.';

  @override
  String get posCartTitle => 'Cart';

  @override
  String get posCartEmpty => 'Cart is empty. Pick items to start.';

  @override
  String posItemsCount(int count) {
    return '$count items';
  }

  @override
  String get posSubtotal => 'Subtotal';

  @override
  String get posDiscount => 'Discount';

  @override
  String get posTaxLine => 'Tax';

  @override
  String get posOtherCost => 'Other costs';

  @override
  String get posTotal => 'Total';

  @override
  String get posPay => 'Pay';

  @override
  String get posTaxToggle => 'Apply tax';

  @override
  String get posClearCart => 'Clear cart';

  @override
  String get posRemoveLine => 'Remove';

  @override
  String posStock(String qty) {
    return 'Stock $qty';
  }

  @override
  String posStockShort(String qty) {
    return 'Not enough stock ($qty available)';
  }

  @override
  String get posBelowCost => 'Price below cost';

  @override
  String get posQuoteFailed => 'Could not calculate totals.';

  @override
  String get posPanelOutlet => 'Outlet';

  @override
  String get posPanelCashier => 'Cashier';

  @override
  String get posPanelBusiness => 'Business';

  @override
  String get posPanelShift => 'Shift';

  @override
  String get posPanelToggle => 'Outlet info';

  @override
  String get posBackHome => 'Back to home';

  @override
  String posShiftOpenedAt(String time) {
    return 'Opened $time';
  }

  @override
  String get posShiftNone => 'No shift yet';

  @override
  String get shiftOpenTitle => 'Open shift';

  @override
  String get shiftOpenHint =>
      'Enter the opening cash in the drawer before you start selling.';

  @override
  String get shiftOpeningCash => 'Opening cash (Rp)';

  @override
  String get shiftOpenSubmit => 'Open shift';

  @override
  String get payTitle => 'Payment';

  @override
  String get payMethod => 'Method';

  @override
  String get payAmount => 'Amount (Rp)';

  @override
  String get payExact => 'Exact amount';

  @override
  String get payAddMethod => 'Add method';

  @override
  String get payRef => 'Reference no.';

  @override
  String get payTotalDue => 'Amount due';

  @override
  String get payPaid => 'Paid';

  @override
  String get payChange => 'Change';

  @override
  String payShortBy(String amount) {
    return 'Short by $amount';
  }

  @override
  String get paySubmit => 'Complete payment';

  @override
  String get payProcessing => 'Processing...';

  @override
  String get paySuccessTitle => 'Sale completed';

  @override
  String paySuccessDoc(String docNo) {
    return 'Receipt no. $docNo';
  }

  @override
  String get payNewSale => 'New sale';

  @override
  String payChangeDue(String amount) {
    return 'Change $amount';
  }

  @override
  String get errorShiftRequired => 'Open a shift before selling.';

  @override
  String get errorStockInsufficient => 'Not enough stock for one of the items.';

  @override
  String get errorValidation => 'Some fields are invalid. Please check.';

  @override
  String get errorForbidden => 'You do not have permission for this action.';

  @override
  String get errorIdempotencyMismatch =>
      'The request conflicts with a previous transaction. Try again.';

  @override
  String get errorMethodInactive => 'The payment method is inactive.';

  @override
  String get errorEditWindow => 'Outside the edit window.';
}
