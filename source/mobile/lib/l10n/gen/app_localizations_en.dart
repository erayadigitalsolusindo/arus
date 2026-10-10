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

  @override
  String get serverTitle => 'Server address';

  @override
  String get serverHelp =>
      'ARUS API address. On a physical phone use your computer\'s IP on the same network, e.g. http://192.168.1.10:8080. Leave empty to use the default.';

  @override
  String get serverLabel => 'Server address';

  @override
  String get serverInvalid =>
      'Must start with http:// or https:// and include a host.';

  @override
  String get save => 'Save';

  @override
  String get posCartEmptyTitle => 'Your cart is empty';

  @override
  String get posCartEmptyHint => 'Pick items to start selling';

  @override
  String get languageTooltip => 'Language';

  @override
  String get languageSystem => 'Follow phone';

  @override
  String get outletSwitch => 'Switch outlet';

  @override
  String get outletSwitchTitle => 'Choose outlet';

  @override
  String outletSwitched(String name) {
    return 'Now at $name';
  }

  @override
  String get outletApprovalTitle => 'Outlet switch approval';

  @override
  String outletApprovalBody(String name) {
    return 'Switching to $name from the register needs Owner/Supervisor approval. The current cart will be cleared.';
  }

  @override
  String get outletApprovalNone => 'No approver at the destination outlet.';

  @override
  String get outletApprover => 'Approver';

  @override
  String get outletPin => 'Approver PIN (6 digits)';

  @override
  String get outletApprovalConfirm => 'Approve & switch';

  @override
  String get errorPinRequired => 'Owner/Supervisor approval (PIN) is required.';

  @override
  String get errorInvalidPin => 'Wrong approver or PIN.';

  @override
  String get errorPinLocked =>
      'PIN is temporarily locked after too many attempts. Try again later.';

  @override
  String get posShiftCloseTooltip => 'Close shift';

  @override
  String get posSalesTodayTooltip => 'Today\'s sales';

  @override
  String shiftCloseTitle(String doc) {
    return 'Close shift $doc';
  }

  @override
  String get shiftCloseBody =>
      'Count the physical money for each method and enter it below. Any difference needs a note and Owner/Supervisor approval.';

  @override
  String shiftCloseOpened(String time) {
    return 'Opened $time';
  }

  @override
  String shiftCloseSales(String count, String total) {
    return '$count sales, $total';
  }

  @override
  String shiftCloseVoid(String count) {
    return '$count voided';
  }

  @override
  String shiftCloseReceivable(String amount) {
    return 'Receivable $amount';
  }

  @override
  String get shiftColExpected => 'Expected';

  @override
  String get shiftColCounted => 'Counted (Rp)';

  @override
  String get shiftColDiff => 'Difference';

  @override
  String get shiftFillExpected => 'Fill with expected';

  @override
  String get shiftTotalDiff => 'Total difference';

  @override
  String get shiftNote => 'Difference note';

  @override
  String get shiftNoteHint => 'Explain the difference (min. 3 characters)';

  @override
  String get shiftNeedApproval =>
      'There is a difference: Owner/Supervisor approval is required.';

  @override
  String get shiftApprovalNone => 'No approver at this outlet.';

  @override
  String get shiftCloseSubmit => 'Close shift';

  @override
  String get shiftChangedNotice =>
      'New sales came in since the recap was shown. It has been reloaded; check your count again.';

  @override
  String shiftClosedTitle(String doc) {
    return 'Shift $doc closed';
  }

  @override
  String get shiftClosedBody => 'The recap is frozen and saved on the server.';

  @override
  String get shiftTotalExpected => 'Expected';

  @override
  String get shiftTotalCounted => 'Counted';

  @override
  String get shiftOpenNew => 'Open new shift';

  @override
  String get shiftDone => 'Done';

  @override
  String get errorShiftRecapChanged =>
      'New sales came in since the recap was shown. Reload the recap and count again.';

  @override
  String get errorShiftDiffNote => 'A difference requires a note and approval.';

  @override
  String get errorShiftClosed => 'This shift is already closed.';

  @override
  String get errorShiftAlreadyOpen =>
      'Your shift at this outlet is already open.';

  @override
  String get salesTodayTitle => 'Today\'s sales';

  @override
  String salesTodaySummary(String count, String total) {
    return '$count sales, $total';
  }

  @override
  String get salesTodaySearch => 'Search receipt number';

  @override
  String get salesTodayEmpty => 'No sales yet.';

  @override
  String get salesTodayTruncated => 'List truncated; narrow it with search.';

  @override
  String get saleVoidBadge => 'VOID';

  @override
  String saleReturnedBadge(String amount) {
    return 'Returned $amount';
  }

  @override
  String get saleCreditBadge => 'Credit';

  @override
  String saleDetailCashier(String name) {
    return 'Cashier $name';
  }

  @override
  String saleDetailMember(String name) {
    return 'Member $name';
  }

  @override
  String saleVoidReason(String reason) {
    return 'Void reason: $reason';
  }

  @override
  String get saleSubtotal => 'Subtotal';

  @override
  String get saleDiscount => 'Discount';

  @override
  String get saleTax => 'Tax';

  @override
  String get saleOtherCost => 'Other charges';

  @override
  String get saleTotal => 'Total';

  @override
  String get salePaid => 'Paid';

  @override
  String get saleChange => 'Change';

  @override
  String get saleReceivable => 'Receivable';

  @override
  String get posScanTooltip => 'Scan barcode with camera';

  @override
  String get scanTitle => 'Scan items';

  @override
  String get scanTorch => 'Torch';

  @override
  String get scanSwitchCamera => 'Switch camera';

  @override
  String get scanHint => 'Point the camera at an item barcode';

  @override
  String get scanDone => 'Done';

  @override
  String get scanPermissionDenied =>
      'Camera permission denied. Allow camera access for ARUS in phone settings.';

  @override
  String get scanCameraError => 'The camera could not be opened.';

  @override
  String scanAdded(String name) {
    return '$name added to cart';
  }

  @override
  String scanNotFound(String code) {
    return 'Code $code not found';
  }

  @override
  String scanAmbiguous(String code) {
    return 'Code $code matches several items; search manually';
  }

  @override
  String get memberChoose => 'Choose member';

  @override
  String get memberChange => 'Change member';

  @override
  String get memberGeneral => 'Walk-in customer';

  @override
  String get memberSearchHint => 'Search name, code, or phone';

  @override
  String get memberSearchEmpty => 'No member found.';

  @override
  String get memberRemove => 'Remove member';

  @override
  String memberPoints(int points) {
    return '$points points';
  }

  @override
  String memberPointsAndDeposit(int points, String deposit) {
    return '$points points · deposit $deposit';
  }

  @override
  String memberEarn(int points) {
    return '+$points points';
  }

  @override
  String get memberRedeemTitle => 'Redeem points';

  @override
  String memberRedeemAvailable(int points, String value) {
    return 'Up to $points points can be redeemed (worth $value)';
  }

  @override
  String get memberRedeemField => 'Points to redeem';

  @override
  String get memberRedeemAll => 'All';

  @override
  String get memberRedeemNone =>
      'This member\'s points can\'t be redeemed on this sale yet.';

  @override
  String get memberRedeemReset => 'Cancel redemption';

  @override
  String get memberRedeemDone => 'Apply';

  @override
  String memberRedeemValue(String amount) {
    return 'Discount $amount';
  }

  @override
  String get errorRedeemInvalid =>
      'Redeemed points exceed the limit. Lower the amount.';

  @override
  String payDepositOver(String amount) {
    return 'Deposit exceeds balance ($amount)';
  }

  @override
  String get adjustTitle => 'Change price / discount';

  @override
  String get adjustBody =>
      'Needs approval from an approver (Owner/Supervisor) with their PIN.';

  @override
  String get adjustItem => 'Item';

  @override
  String get adjustListPrice => 'List price';

  @override
  String get adjustTabPrice => 'Change price';

  @override
  String get adjustTabDiscount => 'Discount';

  @override
  String get adjustNewPrice => 'New unit price (Rp)';

  @override
  String get adjustDiscountRp => 'Discount (Rp)';

  @override
  String get adjustDiscountPct => 'Discount (%)';

  @override
  String get adjustUnitTotal => 'Line total';

  @override
  String get adjustUnitPerUnit => 'Per unit';

  @override
  String adjustPercentPreview(String amount) {
    return 'Discount $amount per unit';
  }

  @override
  String get adjustChecking => 'Checking PIN...';

  @override
  String get adjustApply => 'Apply';

  @override
  String get adjustReset => 'Reset to list price';

  @override
  String get adjustEditTooltip => 'Change price / discount';

  @override
  String get adjustBadgePrice => 'Price changed';

  @override
  String adjustBadgeDiscount(String amount) {
    return 'Discount $amount';
  }

  @override
  String adjustApprovedBy(String name) {
    return 'approved by $name';
  }

  @override
  String get paySurcharge => 'Payment method fee';

  @override
  String get payCharged => 'Charged to customer';

  @override
  String payFeeCustomer(String rate, String fee, String amount) {
    return 'Fee $rate = $fee, charged to the customer (customer pays $amount)';
  }

  @override
  String payFeeStore(String rate, String fee) {
    return 'Fee $rate = $fee, borne by the store';
  }

  @override
  String paySurchargeDone(String surcharge, String charged) {
    return 'Method fee $surcharge · charged $charged';
  }

  @override
  String get costsTitle => 'Note & other costs';

  @override
  String get costsHint =>
      'Itemised other costs (max 20), e.g. shipping or packing.';

  @override
  String get costsName => 'Cost name';

  @override
  String get costsAmount => 'Amount (Rp)';

  @override
  String get costsRemove => 'Remove';

  @override
  String get costsAdd => 'Add cost';

  @override
  String get costsTotal => 'Other costs total';

  @override
  String get costsDone => 'Apply';

  @override
  String get costsClear => 'Clear all';

  @override
  String get posCostsButton => 'Note & other costs';

  @override
  String posTaxStore(String pct) {
    return 'Store tax ($pct%)';
  }

  @override
  String posTaxGov(String pct) {
    return 'Government tax ($pct%)';
  }

  @override
  String get posCostUnnamed => 'Other cost';

  @override
  String get payModePay => 'Pay';

  @override
  String get payModeCredit => 'Credit';

  @override
  String get payCreditNoMember =>
      'Credit is for members only. Choose a member in the cart first.';

  @override
  String get payCreditNotNeeded =>
      'The down payment already covers the total; use Pay mode.';

  @override
  String get payCreditDpAdd => 'Add down payment (DP)';

  @override
  String get payCreditReceivable => 'Receivable (unpaid balance)';

  @override
  String get payCreditDue => 'Due';

  @override
  String payCreditDueDays(int days) {
    return '$days days after the sale';
  }

  @override
  String get payCreditNoDue => 'No due date';

  @override
  String get payCreditLimit => 'Credit limit';

  @override
  String get payCreditNoLimit => 'Unlimited';

  @override
  String get payCreditOutstanding => 'Current receivable';

  @override
  String get payCreditAfter => 'Receivable after this sale';

  @override
  String get payCreditOverLimit =>
      'Over the member\'s credit limit. Needs an approver and their PIN.';

  @override
  String get payCreditSameApprover =>
      'The same approver (price/discount change) is used for the credit limit.';

  @override
  String get errorCreditLimit =>
      'Member\'s receivable exceeds the credit limit. Needs approver (PIN) approval.';

  @override
  String paySuccessReceivable(String amount) {
    return 'Receivable $amount';
  }

  @override
  String paySuccessDue(String date) {
    return 'Due $date';
  }

  @override
  String get pendingHoldTitle => 'Hold sale';

  @override
  String get pendingLabel => 'Label (optional)';

  @override
  String get pendingLabelHint => 'Customer name or a description';

  @override
  String get pendingHold => 'Hold';

  @override
  String get pendingHoldTooltip => 'Hold sale';

  @override
  String get pendingListTooltip => 'Pending sales';

  @override
  String pendingFull(int max) {
    return 'Pending list is full (max $max). Open or delete one first.';
  }

  @override
  String pendingSaved(int no) {
    return 'Sale held as Pending $no';
  }

  @override
  String pendingOpened(int no) {
    return 'Pending $no opened';
  }

  @override
  String pendingOpenedSwapped(int no, int saved) {
    return 'Pending $no opened; previous items saved as Pending $saved';
  }

  @override
  String pendingTitle(int count, int max) {
    return 'Pending sales ($count/$max)';
  }

  @override
  String get pendingHint =>
      'Stored on this phone, expires after 24 hours. Prices and stock are recalculated when opened.';

  @override
  String get pendingEmpty => 'No pending sales.';

  @override
  String pendingNo(int no) {
    return 'Pending $no';
  }

  @override
  String pendingLines(int count) {
    return '$count lines';
  }

  @override
  String get pendingDelete => 'Delete';

  @override
  String pendingDeleteAsk(int no) {
    return 'Delete Pending $no?';
  }

  @override
  String get shortcutsEmpty =>
      'No shortcuts yet. Tap the icon on the right to set them up.';

  @override
  String get shortcutsManage => 'Manage item shortcuts';

  @override
  String get shortcutsTitle => 'Item shortcuts';

  @override
  String get shortcutsHint =>
      'Your 16 slots, the same as on the web cashier. Tap a slot to assign an item.';

  @override
  String get shortcutsSlotEmpty => 'Empty — tap to assign an item';

  @override
  String get shortcutsInactive => 'Item inactive';

  @override
  String get shortcutsClear => 'Clear slot';

  @override
  String get salespersonLabel => 'Salesperson';

  @override
  String get salespersonNone => 'General (no salesperson)';

  @override
  String get salespersonSearch => 'Search salesperson';

  @override
  String get salespersonEmpty => 'No salesperson found.';

  @override
  String get payQuickAmounts => 'Cash received';

  @override
  String get categoryAll => 'All';

  @override
  String get noteLabel => 'Sale note';

  @override
  String get noteHint => 'E.g. customer name, hold, delivery address';

  @override
  String homeGreetMorning(String name) {
    return 'Good morning, $name';
  }

  @override
  String homeGreetNoon(String name) {
    return 'Good afternoon, $name';
  }

  @override
  String homeGreetAfternoon(String name) {
    return 'Good evening, $name';
  }

  @override
  String homeGreetNight(String name) {
    return 'Good night, $name';
  }

  @override
  String get homeTodayTitle => 'Today\'s sales';

  @override
  String get homeTodayNotes => 'Receipts';

  @override
  String get homeShiftLabel => 'Shift';

  @override
  String get homeStallHint => 'Tap to start selling';
}
