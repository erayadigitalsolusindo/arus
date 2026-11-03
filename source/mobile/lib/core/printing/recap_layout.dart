import 'package:decimal/decimal.dart';
import 'package:intl/intl.dart';

import '../../features/pos/sales_models.dart';
import '../../l10n/gen/app_localizations.dart';
import '../money/money.dart';
import 'receipt_layout.dart';

/// Label sumber "uang lain" (sama dengan web `pos.today.flows.*`).
String flowLabel(AppLocalizations l, String source) => switch (source) {
  'receivable_payment' => l.flowReceivablePayment,
  'deposit_topup' => l.flowDepositTopup,
  'deposit_withdraw' => l.flowDepositWithdraw,
  'sale_return' => l.flowSaleReturn,
  'payable_payment' => l.flowPayablePayment,
  'purchase_return' => l.flowPurchaseReturn,
  'supplier_credit_cashout' => l.flowSupplierCashout,
  'petty_cash_in' => l.flowPettyIn,
  'petty_cash_out' => l.flowPettyOut,
  _ => source,
};

/// Batas rentang dari server (`YYYY-MM-DD` atau `YYYY-MM-DDTHH:MM`) untuk ditampilkan: tanggal saja bila tanpa jam.
String formatBound(String raw) {
  final dt = DateTime.tryParse(raw);
  if (dt == null) return raw;
  return raw.contains('T')
      ? DateFormat('dd/MM/yyyy HH:mm').format(dt)
      : DateFormat('dd/MM/yyyy').format(dt);
}

/// Rekap penjualan satu rentang waktu untuk dicetak (cek fisik laci + angka digital per metode). Port dari
/// `dailyRecapLines` di `web/src/lib/pos/report-receipts.ts`; ubah keduanya bersamaan.
List<ReceiptLine> recapLines(
  SalesToday d,
  Paper paper,
  AppLocalizations l, {
  required String tenant,
  required String outlet,
  required String cashier,
}) {
  final w = paper.cols;
  final out = <ReceiptLine>[];
  void push(String text, {bool bold = false}) =>
      out.add(ReceiptLine(text, bold: bold));
  void rule() => push('-' * w);
  void centered(String text, {bool bold = false, bool title = false}) {
    for (final s in wrap(text, w)) {
      out.add(ReceiptLine(center(s, w), bold: bold, title: title));
    }
  }

  String money(Decimal v) => formatMoney(v, symbol: false);
  String signed(Decimal v) => '${v > Decimal.zero ? '+' : ''}${money(v)}';
  void row(String k, String v, {bool bold = false}) =>
      push(lr(k, v, w), bold: bold);

  centered(tenant, bold: true, title: true);
  if (outlet.isNotEmpty && outlet != tenant) centered(outlet);
  rule();
  centered(l.recapTitle, bold: true);
  row(l.recapCashier, cashier);
  // Rentang berjam-menit tidak muat satu baris di kertas 58 mm: awal dan akhir dicetak terpisah.
  push(l.salesFrom);
  push('  ${formatBound(d.from)}');
  push(l.salesTo);
  push('  ${formatBound(d.to)}');
  rule();
  row(l.recapCount, '${d.activeCount}');
  if (d.voidCount > 0) row(l.recapVoided, '${d.voidCount}');
  row(l.recapRevenue, money(d.total), bold: true);
  if (d.credit != null && d.credit! > Decimal.zero) {
    row(l.salesCreditRow, money(d.credit!));
  }
  if (d.returnsCount > 0 && d.returnsTotal != null) {
    row(l.salesReturnsRow('${d.returnsCount}'), '-${money(d.returnsTotal!)}');
    row(l.salesNetTotal, money(d.netTotal ?? d.total), bold: true);
  }
  if (d.surcharge != null && d.surcharge! > Decimal.zero) {
    row(l.salesSurchargeRow, money(d.surcharge!));
  }
  rule();
  push(l.recapByMethod, bold: true);
  for (final m in d.byMethod) {
    row('  ${m.name} (${d.notesFor(m.name)})', money(m.amount));
  }
  if (d.flows.isNotEmpty) {
    rule();
    push(l.salesFlowsTitle, bold: true);
    for (final f in d.flows) {
      row('  ${flowLabel(l, f.source)} - ${f.name}', signed(f.amount));
    }
  }
  rule();
  push(l.salesDrawerTitle, bold: true);
  for (final m in d.drawer) {
    row('  ${m.name}', money(m.amount), bold: m.kind == 'cash');
  }
  rule();
  centered(DateFormat('dd/MM/yyyy HH:mm').format(DateTime.now()));
  return out;
}
