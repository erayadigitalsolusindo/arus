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
  String get textSizeTooltip => 'Text size';

  @override
  String get textSizeTitle => 'Text size';

  @override
  String get textSizeSample => 'Sample text: Iced latte Rp 18,000';

  @override
  String get textSizeDecrease => 'Smaller';

  @override
  String get textSizeIncrease => 'Larger';

  @override
  String get textSizeReset => 'Reset';

  @override
  String get textSizeDone => 'Done';

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
  String get posConfirm => 'Confirm';

  @override
  String get posPay => 'Pay';

  @override
  String get posTaxToggle => 'Apply tax';

  @override
  String get posClearCart => 'Clear cart';

  @override
  String get posClearCartAsk => 'Clear the cart?';

  @override
  String get posClearCartAskBody =>
      'Everything in the cart will be removed and can\'t be restored.';

  @override
  String get posClearCartYes => 'Yes, clear';

  @override
  String get posClearCartNo => 'Keep';

  @override
  String get posAddedToCart => 'Added';

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
  String get shiftOpeningCash => 'Opening cash (\$)';

  @override
  String get shiftOpenSubmit => 'Open shift';

  @override
  String get payTitle => 'Payment';

  @override
  String get payMethod => 'Method';

  @override
  String get payAmount => 'Amount (\$)';

  @override
  String get payExact => 'Exact amount';

  @override
  String get payAddMethod => 'Add method';

  @override
  String get payRef => 'Reference no.';

  @override
  String get payEraGenerate => 'Create QRIS code';

  @override
  String get payEraRegenerate => 'Create new QR';

  @override
  String get payEraCreating => 'Creating QR...';

  @override
  String payEraScan(String amount) {
    return 'Ask the customer to scan and pay $amount';
  }

  @override
  String get payEraWaiting =>
      'Waiting for payment... the sale is saved automatically once paid.';

  @override
  String get payEraPaid => 'QRIS payment received';

  @override
  String get payEraFailed => 'QRIS payment failed. Create a new QR.';

  @override
  String get payEraExpired => 'QR expired. Create a new QR.';

  @override
  String get payEraCancelled => 'Payment cancelled. Create a new QR.';

  @override
  String get payEraNeedPaid => 'Create the QR and wait for the payment.';

  @override
  String get payEraForceShow => 'EraPay unreachable? Force save';

  @override
  String get payEraForceHint =>
      'Force save: the sale is saved and flagged \"check with EraPay\", then reconciled automatically. If it turns out unpaid, the sale is voided and stock restored. Owner/Supervisor approval required.';

  @override
  String get payEraPendingTitle => 'QRIS payment not verified yet';

  @override
  String get payEraPendingNotice =>
      'The sale is flagged \"check with EraPay\" and reconciled automatically. If it turns out unpaid, the sale is voided automatically and stock restored.';

  @override
  String get errorEraDisabled =>
      'EraPay integration is not enabled on the server.';

  @override
  String get errorEraNotConfigured =>
      'No EraPay API key is set for this store. Ask the owner to set it under Payment Methods (web).';

  @override
  String get errorEraRejected => 'EraPay rejected this QRIS charge.';

  @override
  String get errorEraUnavailable => 'Cannot reach EraPay, please retry.';

  @override
  String get errorEraNotPaid =>
      'The QRIS payment is unpaid, mismatched, or already used by another sale.';

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
  String get payItemsTitle => 'Items purchased';

  @override
  String get paySubmit => 'Process Payment';

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
      'Choose the ARUS server. To test on a physical phone, type your computer\'s IP on the same network, e.g. http://192.168.1.10:8080.';

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
  String get posSalesTodayTooltip => 'Sales';

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
  String get shiftColCounted => 'Counted (\$)';

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
  String get salesTodayTitle => 'Sales';

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
  String get adjustNewPrice => 'New unit price (\$)';

  @override
  String get adjustDiscountRp => 'Discount (\$)';

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
  String get costsAmount => 'Amount (\$)';

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

  @override
  String get pettyMenu => 'Petty cash';

  @override
  String get pettyTitle => 'Cashier petty cash';

  @override
  String get pettyBody =>
      'Record money taken out of the drawer outside of sales (water refill, parking, etc.) or money added to the drawer. It is counted when you close the shift.';

  @override
  String get pettyOut => 'Cash out';

  @override
  String get pettyIn => 'Cash in';

  @override
  String get pettyAmount => 'Amount';

  @override
  String get pettyCategory => 'Category';

  @override
  String get pettyNote => 'Note';

  @override
  String get pettySave => 'Save';

  @override
  String get pettySaved => 'Petty cash recorded.';

  @override
  String get pettyHistory => 'This shift\'s entries';

  @override
  String get pettyEmpty => 'No petty cash on this shift yet.';

  @override
  String get pettyVoid => 'Void';

  @override
  String get pettyVoided => 'Voided';

  @override
  String get pettyVoidConfirm =>
      'Void this entry? The expected drawer cash will be recalculated.';

  @override
  String pettyApprovedBy(String name) {
    return 'Approved by $name';
  }

  @override
  String pettyNeedApproval(String limit) {
    return 'Cash out above $limit needs Owner/Supervisor approval (PIN).';
  }

  @override
  String get pettyCatOperational => 'Operations';

  @override
  String get pettyCatMeal => 'Meals';

  @override
  String get pettyCatTransport => 'Parking/Transport';

  @override
  String get pettyCatSupplies => 'Supplies';

  @override
  String get pettyCatChange => 'Add change';

  @override
  String get pettyCatMisc => 'Other';

  @override
  String get errorPettyVoided => 'This petty cash entry is already voided.';

  @override
  String get errorNoCashMethod => 'The cash payment method is not available.';

  @override
  String get homePettyHint => 'Record cash in/out of the drawer';

  @override
  String get updateTitle => 'Update available';

  @override
  String updateVersions(String current, String latest) {
    return 'Installed $current → new version $latest';
  }

  @override
  String get updateBody => 'Update ARUS to the latest version to continue.';

  @override
  String get updateNow => 'Update now';

  @override
  String get updCheckTitle => 'Check for updates';

  @override
  String get updChecking => 'Checking for updates…';

  @override
  String get updUpToDate => 'You are up to date';

  @override
  String get updUpToDateBody => 'You are using the latest version of ARUS.';

  @override
  String get updFailedTitle => 'Could not check';

  @override
  String get updPlayUnavailable =>
      'Could not reach Google Play. Check your connection and try again.';

  @override
  String get updPlayBody =>
      'A new version of ARUS is on Google Play. Update to get fixes and new features.';

  @override
  String get updForcedBody => 'This update is required to keep using ARUS.';

  @override
  String get updLater => 'Later';

  @override
  String get updReadyTitle => 'Update ready to install';

  @override
  String get updReadyBody =>
      'The update has been downloaded. Restart the app to install it.';

  @override
  String get updRestart => 'Restart & install';

  @override
  String get updAvailableBadge => 'New version available';

  @override
  String get updateRetry => 'Try again';

  @override
  String get updateDownloading => 'Downloading update…';

  @override
  String get updateInstalling => 'Opening installer…';

  @override
  String get updatePermission =>
      'Allow ARUS to install apps on the screen that just opened, then come back and tap Update now.';

  @override
  String get updateFailed =>
      'The update could not be downloaded. Check your connection and try again.';

  @override
  String appVersion(String version) {
    return 'Version $version';
  }

  @override
  String get serverPresetProd => 'Production';

  @override
  String get serverPresetEmulator => 'Emulator';

  @override
  String get serverPresetCustom => 'Or type another address:';

  @override
  String get posPanelVersion => 'App version';

  @override
  String get printerMenu => 'Receipt printer';

  @override
  String get printerTitle => 'Receipt printer (Bluetooth)';

  @override
  String get printerHint =>
      'Pair the printer first in Android Settings > Bluetooth, then pick it here. These settings apply to this phone only.';

  @override
  String get printerRefresh => 'Reload list';

  @override
  String get printerNonePaired =>
      'No paired Bluetooth devices yet. Pair the printer in Android Settings, then reload.';

  @override
  String get printerPaper => 'Paper width';

  @override
  String get printerPaper58 => '58 mm';

  @override
  String get printerPaper80 => '80 mm';

  @override
  String get printerCut => 'Cut paper automatically';

  @override
  String get printerAuto => 'Print receipt automatically after payment';

  @override
  String get printerTest => 'Test printer';

  @override
  String get printerTestSent => 'Test page sent.';

  @override
  String get printerNotConfigured =>
      'No printer selected. Pick a Bluetooth printer first.';

  @override
  String get printerPermissionDenied =>
      'Bluetooth permission denied. Allow ARUS to use Bluetooth in the app settings.';

  @override
  String get printerBluetoothOff =>
      'Bluetooth is off. Turn it on and try again.';

  @override
  String get printerConnectFailed =>
      'Could not connect to the printer. Make sure it is on and near the phone.';

  @override
  String get printerWriteFailed =>
      'Sending data to the printer failed. Check the printer and try again.';

  @override
  String get printReceipt => 'Print receipt';

  @override
  String get printReprint => 'Reprint';

  @override
  String get printPrinting => 'Printing…';

  @override
  String printFailed(String error) {
    return 'Receipt failed to print: $error';
  }

  @override
  String get receiptNo => 'No';

  @override
  String get receiptDate => 'Date';

  @override
  String get receiptCashier => 'Cashier';

  @override
  String get receiptMember => 'Member';

  @override
  String get receiptSalesperson => 'Sales';

  @override
  String receiptCopy(int n) {
    return '*** COPY #$n ***';
  }

  @override
  String get receiptVoid => '*** VOIDED ***';

  @override
  String get receiptSuperseded => '*** REPLACED BY REVISION ***';

  @override
  String get receiptEraPending => '*** CHECK WITH ERAPAY ***';

  @override
  String get receiptLineDiscount => 'Discount';

  @override
  String get receiptSubtotal => 'Subtotal';

  @override
  String get receiptDiscount => 'Discount';

  @override
  String receiptVoucher(String code) {
    return 'Coupon $code';
  }

  @override
  String receiptRedeem(int points) {
    return 'Redeem $points pts';
  }

  @override
  String receiptTaxStore(String pct) {
    return 'Store tax $pct%';
  }

  @override
  String receiptTaxGov(String pct) {
    return 'Tax $pct%';
  }

  @override
  String get receiptOtherCost => 'Other cost';

  @override
  String get receiptTotal => 'TOTAL';

  @override
  String get receiptSurcharge => 'Payment fee';

  @override
  String get receiptCharged => 'Amount charged';

  @override
  String get receiptReceivable => 'Credit';

  @override
  String get receiptDue => 'Due date';

  @override
  String get receiptChange => 'Change';

  @override
  String get receiptSaved => 'You saved';

  @override
  String get receiptPointsEarned => 'Points earned';

  @override
  String get receiptTestTitle => 'Printer test page';

  @override
  String get salesFrom => 'From';

  @override
  String get salesTo => 'To';

  @override
  String get salesPresetToday => 'Today';

  @override
  String get salesRangeInvalid => 'The end time must be after the start time.';

  @override
  String get salesRangeTooLong => 'The range can be at most 62 days.';

  @override
  String get salesPrintRecap => 'Print recap';

  @override
  String get salesMethodsTitle => 'Report by payment method';

  @override
  String salesMethodNotes(String count) {
    return '$count sales';
  }

  @override
  String get salesDrawerTitle => 'Should be on hand (physical check)';

  @override
  String get salesFlowsTitle => 'Other money through this cashier';

  @override
  String salesReturnsRow(String count) {
    return 'Returns ($count)';
  }

  @override
  String get salesNetTotal => 'Net sales after returns';

  @override
  String get salesCreditRow => 'Receivable';

  @override
  String get salesSurchargeRow => 'Method fee (customer)';

  @override
  String get flowReceivablePayment => 'Receivable payment';

  @override
  String get flowDepositTopup => 'Deposit top-up';

  @override
  String get flowDepositWithdraw => 'Deposit withdrawal';

  @override
  String get flowSaleReturn => 'Sales return refund';

  @override
  String get flowPayablePayment => 'Supplier payment';

  @override
  String get flowPurchaseReturn => 'Purchase return refund';

  @override
  String get flowSupplierCashout => 'Supplier credit cash-out';

  @override
  String get flowPettyIn => 'Petty cash in';

  @override
  String get flowPettyOut => 'Petty cash out';

  @override
  String get recapTitle => 'SALES RECAP';

  @override
  String get recapCashier => 'Cashier';

  @override
  String get recapPeriod => 'Period';

  @override
  String get recapCount => 'Sales count';

  @override
  String get recapVoided => 'Voided';

  @override
  String get recapRevenue => 'REVENUE';

  @override
  String get recapByMethod => 'Sales by method';

  @override
  String get itemAddTooltip => 'Add item';

  @override
  String get itemAddTitle => 'Add a new item';

  @override
  String get itemName => 'Item name';

  @override
  String get itemSellPrice => 'Selling price';

  @override
  String get itemCost => 'Cost price';

  @override
  String get itemUnit => 'Unit';

  @override
  String get itemCategory => 'Category (optional)';

  @override
  String get itemCategoryNone => 'No category';

  @override
  String get itemBarcode => 'Barcode (optional)';

  @override
  String get itemKind => 'Type';

  @override
  String get itemKindGoods => 'Goods';

  @override
  String get itemKindService => 'Service';

  @override
  String get itemInitialStock => 'Opening stock (optional)';

  @override
  String get itemAllowNegative => 'Allow negative stock';

  @override
  String get itemAllowNegativeHint =>
      'The item can be sold even before stock is entered; stock goes negative until a purchase is recorded.';

  @override
  String get itemStockHint =>
      'Without opening stock or negative stock, the item cannot be sold until stock is added.';

  @override
  String get itemServiceHint => 'Services do not use stock.';

  @override
  String get itemSave => 'Save';

  @override
  String get itemSaveAndSell => 'Save & sell';

  @override
  String itemSaved(String name) {
    return '$name added';
  }

  @override
  String get itemRequired => 'Required';

  @override
  String get itemInvalidNumber => 'Invalid number';

  @override
  String get itemNoUnits => 'No units yet. Add a unit on the web first.';

  @override
  String get itemScanBarcode => 'Scan barcode';

  @override
  String get itemScanned => 'Barcode read';

  @override
  String get itemLoadFailed => 'Could not load units/categories.';

  @override
  String get reportTitle => 'Sales report';

  @override
  String get reportHint => 'Revenue, payment methods, by day';

  @override
  String get reportToday => 'Today';

  @override
  String get report7Days => '7 days';

  @override
  String get reportThisMonth => 'This month';

  @override
  String get reportLastMonth => 'Last month';

  @override
  String get reportCustom => 'Pick dates';

  @override
  String get reportAllOutlets => 'All branches';

  @override
  String get reportRevenue => 'Revenue';

  @override
  String get reportNotes => 'Sales';

  @override
  String get reportVoid => 'Voided';

  @override
  String get reportReturns => 'Returns';

  @override
  String get reportNet => 'Net after returns';

  @override
  String get reportReceivable => 'Receivable';

  @override
  String get reportByMethod => 'By payment method';

  @override
  String get reportByDay => 'By day';

  @override
  String get reportEmpty => 'No sales in this range.';

  @override
  String reportDayNotes(String count) {
    return '$count sales';
  }

  @override
  String get menuItems => 'Items';

  @override
  String get menuPurchases => 'Purchases';

  @override
  String get itemsTitle => 'Items';

  @override
  String get itemsSearch => 'Search name, code, or barcode';

  @override
  String get itemsEmpty => 'No items yet.';

  @override
  String get itemsTabActive => 'Active';

  @override
  String get itemsTabArchived => 'Archived';

  @override
  String get itemsEdit => 'Edit item';

  @override
  String get itemsArchive => 'Archive';

  @override
  String get itemsRestore => 'Restore';

  @override
  String itemsArchiveAsk(String name) {
    return 'Archive $name? It will not appear at the register; transaction history stays intact.';
  }

  @override
  String itemsArchived(String name) {
    return '$name archived';
  }

  @override
  String itemsRestored(String name) {
    return '$name restored';
  }

  @override
  String itemsUpdated(String name) {
    return '$name updated';
  }

  @override
  String itemsStock(String qty) {
    return 'Stock $qty';
  }

  @override
  String get itemsEditTitle => 'Edit item';

  @override
  String get itemsCostNote => 'Cost price changes through purchases, not here.';

  @override
  String get itemsArchivedHint =>
      'Showing at most 30 archived items; narrow down with search.';

  @override
  String get commonCancel => 'Cancel';

  @override
  String get commonSave => 'Save';

  @override
  String get commonRetry => 'Retry';

  @override
  String get purTitle => 'Purchases';

  @override
  String get purAdd => 'New purchase';

  @override
  String get purCashNote =>
      'Purchases from the phone are always cash and add stock right away.';

  @override
  String get purSupplier => 'Supplier';

  @override
  String get purPickSupplier => 'Choose supplier';

  @override
  String get purSearchSupplier => 'Search suppliers';

  @override
  String get purNoSupplier => 'No supplier found.';

  @override
  String get purDate => 'Date';

  @override
  String get purInvoiceNo => 'Supplier invoice no. (optional)';

  @override
  String get purNote => 'Note (optional)';

  @override
  String get purLines => 'Items purchased';

  @override
  String get purAddItem => 'Add item';

  @override
  String get purSearchItem => 'Search items';

  @override
  String get purQty => 'Qty';

  @override
  String get purPrice => 'Buy price';

  @override
  String get purSubtotal => 'Subtotal';

  @override
  String get purTax => 'Tax';

  @override
  String get purOtherCost => 'Other costs';

  @override
  String get purTotal => 'Cash total';

  @override
  String get purSave => 'Save purchase';

  @override
  String purSaved(String doc) {
    return 'Purchase $doc saved, stock increased';
  }

  @override
  String get purNoLines => 'Add at least one item.';

  @override
  String get purNeedSupplier => 'Choose a supplier first.';

  @override
  String get purBadLine =>
      'Quantity must be above 0 and price cannot be empty.';

  @override
  String get purEmpty => 'No purchases in this range.';

  @override
  String purSummary(String count, String total) {
    return '$count purchases, $total';
  }

  @override
  String get purRange30 => '30 days';

  @override
  String get purDetailTitle => 'Purchase detail';

  @override
  String get purEdit => 'Edit';

  @override
  String get purEditTitle => 'Edit purchase';

  @override
  String get purVoid => 'Void';

  @override
  String purVoidAsk(String doc) {
    return 'Void purchase $doc? Stock will be reduced again.';
  }

  @override
  String get purReason => 'Reason (at least 3 characters)';

  @override
  String get purVoided => 'Purchase voided';

  @override
  String get purEdited => 'Purchase updated';

  @override
  String get purStatusVoid => 'VOID';

  @override
  String get purStatusRevised => 'REVISED';

  @override
  String purVoidReason(String reason) {
    return 'Void reason: $reason';
  }

  @override
  String purBy(String name) {
    return 'Recorded by $name';
  }

  @override
  String get errorPurchaseNotEditable =>
      'This purchase can no longer be changed (voided or replaced by a revision).';

  @override
  String get errorPurchaseHasReturns =>
      'This purchase has returns; cancel the return first.';

  @override
  String get errorItemUnavailable => 'The item is not available for purchase.';

  @override
  String get errorCodeTaken => 'The item code is already used.';

  @override
  String get errorEditWindowClosed => 'The edit window has closed.';

  @override
  String get errorStockReverse =>
      'Not enough stock to reverse this purchase (part of it has been sold).';

  @override
  String get menuSettlement => 'Settlement';

  @override
  String get setTitle => 'EraPay settlement';

  @override
  String get setCheck => 'Check with EraPay';

  @override
  String get setChecking => 'Checking with EraPay...';

  @override
  String get setArus => 'ARUS MONEY';

  @override
  String get setEra => 'ERAPAY MONEY';

  @override
  String get setArusHint => 'QRIS on active sales';

  @override
  String get setEraHint => 'Invoices paid at EraPay';

  @override
  String get setMatchedAll => 'Matched: ARUS money equals EraPay money.';

  @override
  String setDiffMore(String amount) {
    return 'Difference $amount: money paid at EraPay is not recorded in an active sale.';
  }

  @override
  String setDiffLess(String amount) {
    return 'Difference $amount: some QRIS sales are not paid at EraPay.';
  }

  @override
  String setCount(String count) {
    return '$count transactions';
  }

  @override
  String get setFilterAll => 'All';

  @override
  String get setFilterAttention => 'Needs attention';

  @override
  String get setFilterWaiting => 'Waiting';

  @override
  String get setFilterMatched => 'Matched';

  @override
  String get setColArus => 'ARUS';

  @override
  String get setColEra => 'ERAPAY';

  @override
  String get setStatusLabel => 'Status';

  @override
  String get setIdLabel => 'EraPay ID';

  @override
  String get setIdCopied => 'ID copied';

  @override
  String get setNoId => 'No ID yet';

  @override
  String get setStateMatched => 'Matched';

  @override
  String get setStateMatchedSub => 'Paid at EraPay and recorded in a sale';

  @override
  String get setStateWaiting => 'Waiting';

  @override
  String get setStateWaitingSub => 'EraPay has not decided yet (QR not paid)';

  @override
  String get setStateOrphan => 'Not recorded';

  @override
  String get setStateOrphanSub => 'Paid at EraPay but no sale was saved';

  @override
  String get setStateRefund => 'Needs refund';

  @override
  String get setStateRefundSub => 'Sale voided, money is still at EraPay';

  @override
  String get setStateUnpaid => 'Unpaid';

  @override
  String get setStateUnpaidSub => 'Sale exists but EraPay failed or expired';

  @override
  String get setStateClosed => 'Not paid';

  @override
  String get setStateClosedSub => 'Invoice failed or expired, no money moved';

  @override
  String get setEraSucceeded => 'Paid';

  @override
  String get setEraPending => 'Pending';

  @override
  String get setEraFailed => 'Failed';

  @override
  String get setEraExpired => 'Expired';

  @override
  String get setEraCancelled => 'Cancelled';

  @override
  String get setEraUnknown => 'Unknown';

  @override
  String get setNotConfigured =>
      'EraPay is not set up for this store. Add the API key under Payment Methods (web).';

  @override
  String get setEmpty => 'No EraPay QRIS transactions in this range.';

  @override
  String setRefreshed(String count) {
    return 'Statuses updated from EraPay: $count';
  }

  @override
  String get setTruncated =>
      'List cut to the latest 300 transactions; narrow the date range.';

  @override
  String get setNote =>
      'EraPay money = invoices marked paid at EraPay. Payout to your bank follows EraPay schedule and is not available here yet.';
}
