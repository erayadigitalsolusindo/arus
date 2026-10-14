// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Japanese (`ja`).
class AppLocalizationsJa extends AppLocalizations {
  AppLocalizationsJa([String locale = 'ja']) : super(locale);

  @override
  String get appName => 'ARUS';

  @override
  String get loginTitleLine1 => 'レジアクセス';

  @override
  String get loginTitleLine2 => 'ARUS (Aciraba Upgrade System)';

  @override
  String get loginSubtitle => '続行するにはログインしてください';

  @override
  String get loginEmail => 'メールアドレス';

  @override
  String get loginEmailHint => 'you@company.com';

  @override
  String get loginPassword => 'パスワード';

  @override
  String get loginShowPassword => 'パスワードを表示';

  @override
  String get loginHidePassword => 'パスワードを隠す';

  @override
  String get loginRemember => 'ログイン状態を保持';

  @override
  String get loginSubmit => 'ログイン';

  @override
  String get loginEmailRequired => 'メールアドレスは必須です。';

  @override
  String get loginEmailInvalid => 'メールアドレスの形式が正しくありません。';

  @override
  String get loginPasswordRequired => 'パスワードは必須です。';

  @override
  String get loginServer => 'サーバー';

  @override
  String get logout => 'ログアウト';

  @override
  String homeWelcome(String name) {
    return 'ようこそ、$nameさん';
  }

  @override
  String get homeOutlet => 'アクティブな店舗';

  @override
  String get homeTenant => '事業';

  @override
  String get homeComingSoon => 'レジ画面を準備しています。';

  @override
  String get errorNetwork => 'サーバーに接続できません。';

  @override
  String get errorTimeout => 'サーバーが応答しません。もう一度お試しください。';

  @override
  String get errorUnknown => 'エラーが発生しました。もう一度お試しください。';

  @override
  String get errorInvalidCredentials => 'メールアドレスまたはパスワードが正しくありません。';

  @override
  String errorInvalidCredentialsLeft(int count) {
    return 'メールアドレスまたはパスワードが正しくありません。あと$count回失敗するとアカウントが一時的にロックされます。';
  }

  @override
  String get errorAccountDisabled => 'このアカウントは無効化されています。管理者に連絡してください。';

  @override
  String get errorNoOutlet => 'このアカウントにはアクティブな店舗がありません。管理者に連絡してください。';

  @override
  String errorAccountLocked(int minutes) {
    return '失敗回数が多すぎます。$minutes分後にもう一度お試しください。';
  }

  @override
  String get errorRateLimited => '試行回数が多すぎます。しばらくしてから再試行してください。';

  @override
  String get errorUnavailable => 'サービスが一時的に利用できません。後でもう一度お試しください。';

  @override
  String get errorInternal => 'サーバーでエラーが発生しました。';

  @override
  String get errorSessionInvalid => 'セッションが終了しました。もう一度ログインしてください。';

  @override
  String get themeSystem => 'システムに従う';

  @override
  String get themeLight => 'ライト';

  @override
  String get themeDark => 'ダーク';

  @override
  String get textSizeTooltip => '文字サイズ';

  @override
  String get textSizeTitle => '文字サイズ';

  @override
  String get textSizeSample => 'サンプルテキスト:ミルクコーヒー Rp 18.000';

  @override
  String get textSizeDecrease => '小さく';

  @override
  String get textSizeIncrease => '大きく';

  @override
  String get textSizeReset => 'リセット';

  @override
  String get textSizeDone => '完了';

  @override
  String themeTooltip(String mode) {
    return 'テーマ: $mode';
  }

  @override
  String get posTitle => 'レジ';

  @override
  String get posSearchHint => '名前、コード、バーコードで検索';

  @override
  String get posSearchEmpty => '商品が見つかりません。';

  @override
  String get posCartTitle => 'カート';

  @override
  String posItemsCount(int count) {
    return '$count 点';
  }

  @override
  String get posSubtotal => '小計';

  @override
  String get posDiscount => '値引き';

  @override
  String get posTaxLine => '消費税';

  @override
  String get posOtherCost => 'その他の費用';

  @override
  String get posTotal => '合計';

  @override
  String get posPay => '支払い';

  @override
  String get posTaxToggle => '消費税を計算';

  @override
  String get posClearCart => 'カートを空にする';

  @override
  String get posRemoveLine => '削除';

  @override
  String posStock(String qty) {
    return '在庫 $qty';
  }

  @override
  String posStockShort(String qty) {
    return '在庫不足（在庫 $qty）';
  }

  @override
  String get posBelowCost => '原価を下回る価格';

  @override
  String get posQuoteFailed => '合計を計算できませんでした。';

  @override
  String get posPanelOutlet => '店舗';

  @override
  String get posPanelCashier => 'キャッシャー';

  @override
  String get posPanelBusiness => '事業';

  @override
  String get posPanelShift => 'シフト';

  @override
  String get posPanelToggle => '店舗情報';

  @override
  String get posBackHome => 'ホームに戻る';

  @override
  String posShiftOpenedAt(String time) {
    return '$time に開始';
  }

  @override
  String get posShiftNone => 'まだシフトがありません';

  @override
  String get shiftOpenTitle => 'シフトを開く';

  @override
  String get shiftOpenHint => '販売を始める前に、レジ引き出しの開始現金を入力してください。';

  @override
  String get shiftOpeningCash => '開始現金 (Rp)';

  @override
  String get shiftOpenSubmit => 'シフトを開く';

  @override
  String get payTitle => '支払い';

  @override
  String get payMethod => '方法';

  @override
  String get payAmount => '金額 (Rp)';

  @override
  String get payExact => 'ちょうどの金額';

  @override
  String get payAddMethod => '方法を追加';

  @override
  String get payRef => '参照番号';

  @override
  String get payEraGenerate => 'QRISコードを作成';

  @override
  String get payEraRegenerate => 'QRを再作成';

  @override
  String get payEraCreating => 'QRを作成中…';

  @override
  String payEraScan(String amount) {
    return 'お客様に $amount のスキャンと支払いをお願いしてください';
  }

  @override
  String get payEraWaiting => '支払い待ち…お支払い完了後にノートが自動保存されます。';

  @override
  String get payEraPaid => 'QRIS支払いを受け付けました';

  @override
  String get payEraFailed => 'QRIS支払いが失敗しました。新しいQRを作成してください。';

  @override
  String get payEraExpired => 'QRの有効期限が切れました。新しいQRを作成してください。';

  @override
  String get payEraCancelled => '支払いがキャンセルされました。新しいQRを作成してください。';

  @override
  String get payEraNeedPaid => 'QRを作成し、支払い完了を待ってください。';

  @override
  String get errorEraDisabled => 'EraPay統合がサーバーで有効になっていません。';

  @override
  String get errorEraNotConfigured =>
      'この店舗にEraPay APIキーが設定されていません。オーナーに支払い方法（Web）で設定してもらってください。';

  @override
  String get errorEraRejected => 'EraPayがこのQRIS請求を拒否しました。';

  @override
  String get errorEraUnavailable => 'EraPayに接続できません。もう一度お試しください。';

  @override
  String get errorEraNotPaid => 'QRIS支払いが未完了、金額不一致、または他のノートに使用済みです。';

  @override
  String get payTotalDue => '請求合計';

  @override
  String get payPaid => '支払い済み';

  @override
  String get payChange => 'お釣り';

  @override
  String payShortBy(String amount) {
    return '$amount 不足';
  }

  @override
  String get paySubmit => '支払いを完了する';

  @override
  String get payProcessing => '処理中…';

  @override
  String get paySuccessTitle => '取引が完了しました';

  @override
  String paySuccessDoc(String docNo) {
    return 'ノート番号 $docNo';
  }

  @override
  String get payNewSale => '新しい取引';

  @override
  String payChangeDue(String amount) {
    return 'お釣り $amount';
  }

  @override
  String get errorShiftRequired => '販売前にシフトを開いてください。';

  @override
  String get errorStockInsufficient => 'いずれかの商品の在庫が不足しています。';

  @override
  String get errorValidation => '入力内容が不正です。確認してください。';

  @override
  String get errorForbidden => 'この操作を行う権限がありません。';

  @override
  String get errorIdempotencyMismatch => 'リクエストが前回の取引と競合しています。もう一度お試しください。';

  @override
  String get errorMethodInactive => 'この支払い方法は無効です。';

  @override
  String get errorEditWindow => '編集可能な期間を超えています。';

  @override
  String get serverTitle => 'サーバーアドレス';

  @override
  String get serverHelp =>
      'ARUSサーバーを選択します。実機スマホでテストするには、同じネットワークのPCのIPアドレスを入力します（例: http://192.168.1.10:8080）。';

  @override
  String get serverLabel => 'サーバーアドレス';

  @override
  String get serverInvalid => 'http:// または https:// で始まり、アドレスが含まれている必要があります。';

  @override
  String get save => '保存';

  @override
  String get posCartEmptyTitle => 'カートは空です';

  @override
  String get posCartEmptyHint => '商品を選んで販売を始めましょう';

  @override
  String get languageTooltip => '言語';

  @override
  String get languageSystem => '端末に従う';

  @override
  String get outletSwitch => '店舗切り替え';

  @override
  String get outletSwitchTitle => '店舗を選択';

  @override
  String outletSwitched(String name) {
    return '現在 $name の店舗です';
  }

  @override
  String get outletApprovalTitle => '店舗切り替えの承認';

  @override
  String outletApprovalBody(String name) {
    return 'レジから $name への切り替えにはオーナー/スーパーバイザーの承認が必要です。現在のカートは空になります。';
  }

  @override
  String get outletApprovalNone => '移動先の店舗に承認者がいません。';

  @override
  String get outletApprover => '承認者';

  @override
  String get outletPin => '承認者PIN（6桁）';

  @override
  String get outletApprovalConfirm => '承認して切り替え';

  @override
  String get errorPinRequired => 'オーナー/スーパーバイザーの承認（PIN）が必要です。';

  @override
  String get errorInvalidPin => '承認者またはPINが正しくありません。';

  @override
  String get errorPinLocked => '試行回数が多すぎたためPINが一時的にロックされています。後でもう一度お試しください。';

  @override
  String get posShiftCloseTooltip => 'シフトを閉じる';

  @override
  String get posSalesTodayTooltip => '本日の売上';

  @override
  String shiftCloseTitle(String doc) {
    return 'シフト $doc を閉じる';
  }

  @override
  String get shiftCloseBody =>
      '各方法の引き出しにある現金を数えて下に入力してください。差額にはメモとオーナー/スーパーバイザーの承認が必要です。';

  @override
  String shiftCloseOpened(String time) {
    return '$time に開始';
  }

  @override
  String shiftCloseSales(String count, String total) {
    return '$count ノート、$total';
  }

  @override
  String shiftCloseVoid(String count) {
    return '$count 件のキャンセル';
  }

  @override
  String shiftCloseReceivable(String amount) {
    return '売掛金 $amount';
  }

  @override
  String get shiftColExpected => '期待値';

  @override
  String get shiftColCounted => '現金（Rp）';

  @override
  String get shiftColDiff => '差額';

  @override
  String get shiftFillExpected => '期待値で埋める';

  @override
  String get shiftTotalDiff => '差額合計';

  @override
  String get shiftNote => '差額メモ';

  @override
  String get shiftNoteHint => '差額の原因を説明（3文字以上）';

  @override
  String get shiftNeedApproval => '差額があります:オーナー/スーパーバイザーの承認が必要です。';

  @override
  String get shiftApprovalNone => 'この店舗に承認者がいません。';

  @override
  String get shiftCloseSubmit => 'シフトを閉じる';

  @override
  String get shiftChangedNotice =>
      'サマリー表示後に新しい取引がありました。サマリーは再読み込み済みです。数え直してください。';

  @override
  String shiftClosedTitle(String doc) {
    return 'シフト $doc を閉じました';
  }

  @override
  String get shiftClosedBody => 'サマリーは固定され、サーバーに保存されました。';

  @override
  String get shiftTotalExpected => '期待値';

  @override
  String get shiftTotalCounted => '数えた額';

  @override
  String get shiftOpenNew => '新しいシフトを開く';

  @override
  String get shiftDone => '完了';

  @override
  String get errorShiftRecapChanged =>
      'サマリー表示後に新しい取引がありました。サマリーを再読み込みしてから数え直してください。';

  @override
  String get errorShiftDiffNote => '差額にはメモと承認が必要です。';

  @override
  String get errorShiftClosed => 'このシフトはすでに閉じています。';

  @override
  String get errorShiftAlreadyOpen => 'この店舗のシフトはすでに開いています。';

  @override
  String get salesTodayTitle => '本日の売上';

  @override
  String salesTodaySummary(String count, String total) {
    return '$count ノート、$total';
  }

  @override
  String get salesTodaySearch => 'ノート番号を検索';

  @override
  String get salesTodayEmpty => 'まだ売上がありません。';

  @override
  String get salesTodayTruncated => 'リストが切り詰められました。検索で絞り込んでください。';

  @override
  String get saleVoidBadge => 'キャンセル';

  @override
  String saleReturnedBadge(String amount) {
    return '返品 $amount';
  }

  @override
  String get saleCreditBadge => '売掛';

  @override
  String saleDetailCashier(String name) {
    return 'キャッシャー $name';
  }

  @override
  String saleDetailMember(String name) {
    return 'メンバー $name';
  }

  @override
  String saleVoidReason(String reason) {
    return 'キャンセル理由: $reason';
  }

  @override
  String get saleSubtotal => '小計';

  @override
  String get saleDiscount => '値引き';

  @override
  String get saleTax => '消費税';

  @override
  String get saleOtherCost => 'その他の費用';

  @override
  String get saleTotal => '合計';

  @override
  String get salePaid => '支払い済み';

  @override
  String get saleChange => 'お釣り';

  @override
  String get saleReceivable => '売掛金';

  @override
  String get posScanTooltip => 'カメラでバーコードをスキャン';

  @override
  String get scanTitle => '商品をスキャン';

  @override
  String get scanTorch => 'ライト';

  @override
  String get scanSwitchCamera => 'カメラ切り替え';

  @override
  String get scanHint => 'カメラを商品のバーコードに向けてください';

  @override
  String get scanDone => '完了';

  @override
  String get scanPermissionDenied =>
      'カメラの権限が拒否されました。スマホの設定でARUSにカメラの使用を許可してください。';

  @override
  String get scanCameraError => 'カメラを開けませんでした。';

  @override
  String scanAdded(String name) {
    return '$name をカートに追加しました';
  }

  @override
  String scanNotFound(String code) {
    return 'コード $code は見つかりません';
  }

  @override
  String scanAmbiguous(String code) {
    return 'コード $code が複数の商品と一致します。手動で検索してください';
  }

  @override
  String get memberChoose => 'メンバーを選択';

  @override
  String get memberChange => 'メンバーを変更';

  @override
  String get memberGeneral => '一般のお客様';

  @override
  String get memberSearchHint => '名前、コード、電話番号で検索';

  @override
  String get memberSearchEmpty => 'メンバーが見つかりません。';

  @override
  String get memberRemove => 'メンバーを外す';

  @override
  String memberPoints(int points) {
    return '$points ポイント';
  }

  @override
  String memberPointsAndDeposit(int points, String deposit) {
    return '$points ポイント・デポジット $deposit';
  }

  @override
  String memberEarn(int points) {
    return '+$points ポイント';
  }

  @override
  String get memberRedeemTitle => 'ポイント交換';

  @override
  String memberRedeemAvailable(int points, String value) {
    return '最大 $points ポイント交換できます（価値 $value）';
  }

  @override
  String get memberRedeemField => '交換するポイント';

  @override
  String get memberRedeemAll => 'すべて';

  @override
  String get memberRedeemNone => 'このメンバーのポイントはこのノートでまだ交換できません。';

  @override
  String get memberRedeemReset => '交換をキャンセル';

  @override
  String get memberRedeemDone => '適用';

  @override
  String memberRedeemValue(String amount) {
    return '値引き $amount';
  }

  @override
  String get errorRedeemInvalid => '交換ポイントが上限を超えています。数を減らしてください。';

  @override
  String payDepositOver(String amount) {
    return 'デポジットが残高を超えています（$amount）';
  }

  @override
  String get adjustTitle => '価格/値引きの変更';

  @override
  String get adjustBody => '承認者（オーナー/スーパーバイザー）のPINによる承認が必要です。';

  @override
  String get adjustItem => '商品';

  @override
  String get adjustListPrice => '通常価格';

  @override
  String get adjustTabPrice => '価格変更';

  @override
  String get adjustTabDiscount => '値引き';

  @override
  String get adjustNewPrice => '新しい単価 (Rp)';

  @override
  String get adjustDiscountRp => '値引き (Rp)';

  @override
  String get adjustDiscountPct => '値引き (%)';

  @override
  String get adjustUnitTotal => '行の合計';

  @override
  String get adjustUnitPerUnit => '単価あたり';

  @override
  String adjustPercentPreview(String amount) {
    return '単価あたりの値引き $amount';
  }

  @override
  String get adjustChecking => 'PINを確認中…';

  @override
  String get adjustApply => '適用';

  @override
  String get adjustReset => '通常価格に戻す';

  @override
  String get adjustEditTooltip => '価格/値引きの変更';

  @override
  String get adjustBadgePrice => '価格変更';

  @override
  String adjustBadgeDiscount(String amount) {
    return '値引き $amount';
  }

  @override
  String adjustApprovedBy(String name) {
    return '$name が承認';
  }

  @override
  String get paySurcharge => '支払い方法の手数料';

  @override
  String get payCharged => 'お客様に請求';

  @override
  String payFeeCustomer(String rate, String fee, String amount) {
    return '手数料 $rate = $fee、お客様に請求されます（お客様は $amount 支払います）';
  }

  @override
  String payFeeStore(String rate, String fee) {
    return '手数料 $rate = $fee、店舗負担';
  }

  @override
  String paySurchargeDone(String surcharge, String charged) {
    return '方法手数料 $surcharge・請求 $charged';
  }

  @override
  String get costsTitle => 'メモとその他の費用';

  @override
  String get costsHint => '明細付きのその他の費用（最大20）、例:送料や梱包費。';

  @override
  String get costsName => '費用名';

  @override
  String get costsAmount => '金額 (Rp)';

  @override
  String get costsRemove => '削除';

  @override
  String get costsAdd => '費用を追加';

  @override
  String get costsTotal => 'その他の費用合計';

  @override
  String get costsDone => '適用';

  @override
  String get costsClear => 'すべて削除';

  @override
  String get posCostsButton => 'メモとその他の費用';

  @override
  String posTaxStore(String pct) {
    return '店舗税 ($pct%)';
  }

  @override
  String posTaxGov(String pct) {
    return '政府税 ($pct%)';
  }

  @override
  String get posCostUnnamed => 'その他の費用';

  @override
  String get payModePay => '支払い';

  @override
  String get payModeCredit => '掛け売り';

  @override
  String get payCreditNoMember => '掛け売りはメンバー限定です。先にカートでメンバーを選択してください。';

  @override
  String get payCreditNotNeeded => '頭金が合計を支払っています。「支払い」モードを使用してください。';

  @override
  String get payCreditDpAdd => '頭金（DP）を追加';

  @override
  String get payCreditReceivable => '売掛金（未払残高）';

  @override
  String get payCreditDue => '支払期日';

  @override
  String payCreditDueDays(int days) {
    return 'ノートから $days 日';
  }

  @override
  String get payCreditNoDue => '期日なし';

  @override
  String get payCreditLimit => '与信限度';

  @override
  String get payCreditNoLimit => '無制限';

  @override
  String get payCreditOutstanding => '現在の売掛金';

  @override
  String get payCreditAfter => 'このノート後の売掛金';

  @override
  String get payCreditOverLimit => 'メンバーの与信限度を超えています。承認者とそのPINが必要です。';

  @override
  String get payCreditSameApprover => '認証者（価格/値引き変更）と同じ与信限度承認者を使用します。';

  @override
  String get errorCreditLimit => 'メンバーの売掛金が与信限度を超えています。承認者（PIN）の承認が必要です。';

  @override
  String paySuccessReceivable(String amount) {
    return '売掛金 $amount';
  }

  @override
  String paySuccessDue(String date) {
    return '支払期日 $date';
  }

  @override
  String get pendingHoldTitle => 'ノートを保留';

  @override
  String get pendingLabel => 'ラベル（任意）';

  @override
  String get pendingLabelHint => 'お客様名または特徴';

  @override
  String get pendingHold => '保留';

  @override
  String get pendingHoldTooltip => 'ノートを保留';

  @override
  String get pendingListTooltip => '保留中のノート';

  @override
  String pendingFull(int max) {
    return '保留リストがいっぱいです（最大 $max）。先に1つ開くか削除してください。';
  }

  @override
  String pendingSaved(int no) {
    return 'ノートを Pending $no として保留しました';
  }

  @override
  String pendingOpened(int no) {
    return 'Pending $no を開きました';
  }

  @override
  String pendingOpenedSwapped(int no, int saved) {
    return 'Pending $no を開きました;以前の内容は Pending $saved として保存されました';
  }

  @override
  String pendingTitle(int count, int max) {
    return '保留中のノート（$count/$max）';
  }

  @override
  String get pendingHint => 'このスマホに保存され、24時間で期限切れになります。開くと価格と在庫が再計算されます。';

  @override
  String get pendingEmpty => '保留中のノートがありません。';

  @override
  String pendingNo(int no) {
    return 'Pending $no';
  }

  @override
  String pendingLines(int count) {
    return '$count 行';
  }

  @override
  String get pendingDelete => '削除';

  @override
  String pendingDeleteAsk(int no) {
    return 'Pending $no を削除しますか？';
  }

  @override
  String get shortcutsEmpty => 'まだショートカットがありません。右のアイコンをタップして設定してください。';

  @override
  String get shortcutsManage => '商品ショートカットを管理';

  @override
  String get shortcutsTitle => '商品ショートカット';

  @override
  String get shortcutsHint => 'あなたの16スロット（Webレジと同じ）です。スロットをタップして商品を割り当ててください。';

  @override
  String get shortcutsSlotEmpty => '空 — タップして商品を割り当て';

  @override
  String get shortcutsInactive => '無効な商品';

  @override
  String get shortcutsClear => 'スロットを空にする';

  @override
  String get salespersonLabel => '営業担当';

  @override
  String get salespersonNone => '一般（営業担当なし）';

  @override
  String get salespersonSearch => '営業担当を検索';

  @override
  String get salespersonEmpty => '営業担当が見つかりません。';

  @override
  String get payQuickAmounts => '受け取った現金';

  @override
  String get categoryAll => 'すべて';

  @override
  String get noteLabel => 'ノートのメモ';

  @override
  String get noteHint => '例:お客様名、預かり、配送先住所';

  @override
  String homeGreetMorning(String name) {
    return 'おはようございます、$nameさん';
  }

  @override
  String homeGreetNoon(String name) {
    return 'こんにちは、$nameさん';
  }

  @override
  String homeGreetAfternoon(String name) {
    return 'こんばんは、$nameさん';
  }

  @override
  String homeGreetNight(String name) {
    return 'おやすみなさい、$nameさん';
  }

  @override
  String get homeTodayTitle => '本日の売上';

  @override
  String get homeTodayNotes => 'ノート';

  @override
  String get homeShiftLabel => 'シフト';

  @override
  String get homeStallHint => 'タップして販売を開始';

  @override
  String get pettyMenu => '小口現金';

  @override
  String get pettyTitle => 'キャッシャー小口現金';

  @override
  String get pettyBody =>
      '販売以外で引き出しから出たお金（水の補充、駐車場など）や引き出しに追加したお金を記録します。シフトを閉じるときに計算されます。';

  @override
  String get pettyOut => '現金払出';

  @override
  String get pettyIn => '現金投入';

  @override
  String get pettyAmount => '金額';

  @override
  String get pettyCategory => 'カテゴリ';

  @override
  String get pettyNote => 'メモ';

  @override
  String get pettySave => '保存';

  @override
  String get pettySaved => '小口現金を記録しました。';

  @override
  String get pettyHistory => 'このシフトの記録';

  @override
  String get pettyEmpty => 'このシフトにはまだ小口現金がありません。';

  @override
  String get pettyVoid => 'キャンセル';

  @override
  String get pettyVoided => 'キャンセル済み';

  @override
  String get pettyVoidConfirm => 'この記録をキャンセルしますか？期待される引き出し現金が再計算されます。';

  @override
  String pettyApprovedBy(String name) {
    return '$name が承認';
  }

  @override
  String pettyNeedApproval(String limit) {
    return '$limit を超える現金払出にはオーナー/スーパーバイザーの承認（PIN）が必要です。';
  }

  @override
  String get pettyCatOperational => '運営';

  @override
  String get pettyCatMeal => '飲食';

  @override
  String get pettyCatTransport => '駐車場/移動';

  @override
  String get pettyCatSupplies => '消耗品';

  @override
  String get pettyCatChange => '釣り銭補充';

  @override
  String get pettyCatMisc => 'その他';

  @override
  String get errorPettyVoided => 'この小口現金記録はすでにキャンセルされています。';

  @override
  String get errorNoCashMethod => '現金支払い方法が利用できません。';

  @override
  String get homePettyHint => '引き出しの入出金を記録';

  @override
  String get updateTitle => 'アップデートがあります';

  @override
  String updateVersions(String current, String latest) {
    return 'インストール済み $current → 新バージョン $latest';
  }

  @override
  String get updateBody => 'ARUSを最新バージョンに更新して続行してください。';

  @override
  String get updateNow => '今すぐ更新';

  @override
  String get updateRetry => '再試行';

  @override
  String get updateDownloading => 'アップデートをダウンロード中…';

  @override
  String get updateInstalling => 'インストーラーを開いています…';

  @override
  String get updatePermission =>
      '開いた画面でARUSにアプリのインストールを許可し、ここに戻って「今すぐ更新」をタップしてください。';

  @override
  String get updateFailed => 'アップデートをダウンロードできませんでした。接続を確認して再試行してください。';

  @override
  String appVersion(String version) {
    return 'バージョン $version';
  }

  @override
  String get serverPresetProd => '本番';

  @override
  String get serverPresetEmulator => 'エミュレーター';

  @override
  String get serverPresetCustom => 'または他のアドレスを入力:';

  @override
  String get posPanelVersion => 'アプリバージョン';

  @override
  String get printerMenu => 'レシートプリンター';

  @override
  String get printerTitle => 'レシートプリンター（Bluetooth）';

  @override
  String get printerHint =>
      '先にAndroid設定>Bluetoothでプリンターをペアリングし、ここで選択してください。この設定はこのスマホだけに適用されます。';

  @override
  String get printerRefresh => 'リストを再読み込み';

  @override
  String get printerNonePaired =>
      'ペアリング済みのBluetoothデバイスがまだありません。Android設定でプリンターをペアリングしてから再読み込みしてください。';

  @override
  String get printerPaper => '用紙幅';

  @override
  String get printerPaper58 => '58 mm';

  @override
  String get printerPaper80 => '80 mm';

  @override
  String get printerCut => '用紙を自動カット';

  @override
  String get printerAuto => '支払い後にレシートを自動印刷';

  @override
  String get printerTest => 'プリンターをテスト';

  @override
  String get printerTestSent => 'テストページを送信しました。';

  @override
  String get printerNotConfigured =>
      'プリンターが選択されていません。先にBluetoothプリンターを選択してください。';

  @override
  String get printerPermissionDenied =>
      'Bluetoothの権限が拒否されました。アプリ設定でARUSにBluetoothの使用を許可してください。';

  @override
  String get printerBluetoothOff => 'Bluetoothがオフです。オンにしてから再試行してください。';

  @override
  String get printerConnectFailed =>
      'プリンターに接続できません。プリンターの電源が入っていて、スマホの近くにあることを確認してください。';

  @override
  String get printerWriteFailed => 'プリンターへのデータ送信に失敗しました。プリンターを確認してから再試行してください。';

  @override
  String get printReceipt => 'レシートを印刷';

  @override
  String get printReprint => '再印刷';

  @override
  String get printPrinting => '印刷中…';

  @override
  String printFailed(String error) {
    return 'レシート印刷に失敗しました: $error';
  }

  @override
  String get receiptNo => '番号';

  @override
  String get receiptDate => '日付';

  @override
  String get receiptCashier => 'キャッシャー';

  @override
  String get receiptMember => 'メンバー';

  @override
  String get receiptSalesperson => '営業';

  @override
  String receiptCopy(int n) {
    return '*** $n 枚目のコピー ***';
  }

  @override
  String get receiptVoid => '*** キャンセル済み ***';

  @override
  String get receiptSuperseded => '*** 改訂で差し替え ***';

  @override
  String get receiptLineDiscount => '値引き';

  @override
  String get receiptSubtotal => '小計';

  @override
  String get receiptDiscount => '割引';

  @override
  String receiptVoucher(String code) {
    return 'クーポン $code';
  }

  @override
  String receiptRedeem(int points) {
    return '$points ポイント交換';
  }

  @override
  String receiptTaxStore(String pct) {
    return '店舗税 $pct%';
  }

  @override
  String receiptTaxGov(String pct) {
    return '税 $pct%';
  }

  @override
  String get receiptOtherCost => 'その他の費用';

  @override
  String get receiptTotal => '合計';

  @override
  String get receiptSurcharge => '支払い手数料';

  @override
  String get receiptCharged => '請求合計';

  @override
  String get receiptReceivable => '売掛';

  @override
  String get receiptDue => '支払期日';

  @override
  String get receiptChange => 'お釣り';

  @override
  String get receiptSaved => 'お得額';

  @override
  String get receiptPointsEarned => '獲得ポイント';

  @override
  String get receiptTestTitle => 'プリンターのテストページ';
}
