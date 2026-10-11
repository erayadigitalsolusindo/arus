import 'package:decimal/decimal.dart';
import 'package:intl/intl.dart';

import '../../l10n/gen/app_localizations.dart';
import '../money/money.dart';
import 'receipt_model.dart';

/// Template struk — port dari `web/src/lib/pos/receipt.ts` (`receiptLines`): baris teks berlebar tetap
/// (32 kolom = kertas 58 mm, 48 kolom = 80 mm). Ubah susunan struk di kedua tempat sekaligus agar web dan HP sama.
/// [title] = nama toko (ESC/POS: rata tengah + ukuran ganda bila muat).
class ReceiptLine {
  const ReceiptLine(
    this.text, {
    this.bold = false,
    this.big = false,
    this.title = false,
  });

  final String text;
  final bool bold;
  final bool big;
  final bool title;
}

enum Paper {
  mm58(58, 32),
  mm80(80, 48);

  const Paper(this.mm, this.cols);

  final int mm;
  final int cols;
}

/// Pecah teks panjang menjadi beberapa baris selebar [w] (per kata; kata yang terlalu panjang dipotong).
List<String> wrap(String text, int w) {
  final out = <String>[];
  for (final para in text.split('\n')) {
    var cur = '';
    for (final word in para.split(RegExp(r'\s+')).where((s) => s.isNotEmpty)) {
      var wd = word;
      while (wd.length > w) {
        if (cur.isNotEmpty) {
          out.add(cur);
          cur = '';
        }
        out.add(wd.substring(0, w));
        wd = wd.substring(w);
      }
      if (cur.isEmpty) {
        cur = wd;
      } else if (cur.length + 1 + wd.length <= w) {
        cur += ' $wd';
      } else {
        out.add(cur);
        cur = wd;
      }
    }
    out.add(cur);
  }
  return out;
}

String center(String s, int w) =>
    ' ' * ((w - s.length) ~/ 2).clamp(0, w).toInt() + s;

/// Label kiri + nilai kanan dalam satu baris; label dipotong bila tidak muat.
String lr(String left, String right, int w) {
  final room = w - right.length - 1;
  if (room < 1) return right.padLeft(w);
  final l = left.length > room ? left.substring(0, room) : left;
  return l + ' ' * (w - l.length - right.length) + right;
}

/// Susun struk menjadi baris teks. [copy] > 0 = cetak ulang ke-n (ditandai di struk).
List<ReceiptLine> receiptLines(
  ReceiptData rc,
  Paper paper,
  AppLocalizations l, {
  int copy = 0,
}) {
  final w = paper.cols;
  final nf = NumberFormat.decimalPattern(l.localeName);
  final decSep = nf.symbols.DECIMAL_SEP;
  String amt(Decimal d) {
    final neg = d < Decimal.zero;
    final abs = neg ? -d : d;
    final whole = nf.format(abs.truncate().toBigInt().toInt());
    final frac = (abs - abs.truncate()).toStringAsFixed(2).substring(2);
    final s = frac == '00' ? whole : '$whole$decSep$frac';
    return neg ? '-$s' : s;
  }

  String qty(Decimal d) => decToApi(d).replaceFirst('.', decSep);

  final out = <ReceiptLine>[];
  void push(String text, {bool bold = false, bool big = false}) =>
      out.add(ReceiptLine(text, bold: bold, big: big));
  void rule() => push('-' * w);
  void centered(String text, {bool bold = false, bool title = false}) {
    for (final s in wrap(text, w)) {
      out.add(ReceiptLine(center(s, w), bold: bold, title: title));
    }
  }

  final store = rc.store;
  centered(store.tenantName, bold: true, title: true);
  if (store.outletName.isNotEmpty && store.outletName != store.tenantName) {
    centered(store.outletName);
  }
  if (store.address.isNotEmpty) centered(store.address);
  if (store.phone.isNotEmpty) centered(store.phone);
  if (store.header.isNotEmpty) centered(store.header);
  rule();

  push(lr(l.receiptNo, rc.docNo, w));
  push(lr(l.receiptDate, rc.localTime, w));
  push(lr(l.receiptCashier, rc.cashier, w));
  if (rc.member.isNotEmpty) push(lr(l.receiptMember, rc.member, w));
  if (rc.salesperson.isNotEmpty) {
    push(lr(l.receiptSalesperson, rc.salesperson, w));
  }
  if (copy > 0) centered(l.receiptCopy(copy), bold: true);
  if (rc.status == 'void') {
    centered(l.receiptVoid, bold: true);
  } else if (rc.status == 'superseded') {
    centered(l.receiptSuperseded, bold: true);
  }
  rule();

  var lineDisc = Decimal.zero;
  for (final it in rc.lines) {
    for (final s in wrap(it.name, w)) {
      push(s);
    }
    push(
      lr(
        '  ${qty(it.qty)} ${it.unit} x ${amt(it.unitPrice)}',
        amt(it.lineTotal),
        w,
      ),
    );
    if (it.discount > Decimal.zero) {
      lineDisc += it.discount;
      push(lr('  ${l.receiptLineDiscount}', '-${amt(it.discount)}', w));
    }
  }
  rule();

  push(lr(l.receiptSubtotal, amt(rc.subtotal), w));
  final voucherSum = rc.vouchers.fold(Decimal.zero, (a, v) => a + v.amount);
  final manual = rc.discount - voucherSum - rc.redeemAmount;
  if (manual > Decimal.zero) {
    push(lr(l.receiptDiscount, '-${amt(manual)}', w));
  }
  for (final v in rc.vouchers) {
    push(lr(l.receiptVoucher(v.code), '-${amt(v.amount)}', w));
  }
  if (rc.redeemAmount > Decimal.zero) {
    push(
      lr(
        l.receiptRedeem(rc.pointsRedeemed),
        '-${amt(rc.redeemAmount)}',
        w,
      ),
    );
  }
  if (rc.taxStore > Decimal.zero) {
    push(lr(l.receiptTaxStore(qty(rc.taxStorePct)), amt(rc.taxStore), w));
  }
  if (rc.taxGov > Decimal.zero) {
    push(lr(l.receiptTaxGov(qty(rc.taxGovPct)), amt(rc.taxGov), w));
  }
  if (rc.otherCosts.isNotEmpty) {
    for (final c in rc.otherCosts) {
      push(lr(c.name, amt(c.amount), w));
    }
  } else if (rc.otherCost > Decimal.zero) {
    push(lr(l.receiptOtherCost, amt(rc.otherCost), w));
  }
  push(lr(l.receiptTotal, amt(rc.total), w), bold: true, big: true);

  if (rc.surcharge > Decimal.zero) {
    push(lr(l.receiptSurcharge, amt(rc.surcharge), w));
    push(lr(l.receiptCharged, amt(rc.total + rc.surcharge), w), bold: true);
  }
  rule();
  for (final p in rc.payments) {
    final fee = p.feeByCustomer ? p.fee : Decimal.zero;
    push(lr(p.methodName, amt(p.amount + fee), w));
    if (p.refNo.isNotEmpty) {
      final s = '  ${p.refNo}';
      push(s.length > w ? s.substring(0, w) : s);
    }
  }
  if (rc.receivable > Decimal.zero) {
    push(lr(l.receiptReceivable, amt(rc.receivable), w), bold: true);
    if (rc.dueDate.isNotEmpty) {
      push(lr(l.receiptDue, rc.dueDate.split('-').reversed.join('/'), w));
    }
  } else {
    push(lr(l.receiptChange, amt(rc.change), w), bold: true);
  }
  final saved = lineDisc + manual + voucherSum + rc.redeemAmount;
  if (saved > Decimal.zero) push(lr(l.receiptSaved, amt(saved), w));
  if (rc.member.isNotEmpty && rc.pointsEarned > 0) {
    push(lr(l.receiptPointsEarned, '+${rc.pointsEarned}', w));
  }

  if (rc.note.isNotEmpty) {
    rule();
    for (final s in wrap(rc.note, w)) {
      push(s);
    }
  }
  if (store.footer.isNotEmpty) {
    rule();
    centered(store.footer);
  }
  return out;
}

/// Struk uji untuk tombol "Tes printer".
List<ReceiptLine> testLines(Paper paper, AppLocalizations l) {
  final w = paper.cols;
  return [
    ReceiptLine(center('ARUS', w), bold: true, title: true),
    ReceiptLine(center(l.receiptTestTitle, w)),
    ReceiptLine('-' * w),
    ReceiptLine(('1234567890' * 5).substring(0, w)),
    ReceiptLine(lr(l.receiptTotal, '123.456', w), bold: true, big: true),
    ReceiptLine('-' * w),
    ReceiptLine(
      center(DateFormat('d MMM yyyy HH:mm', l.localeName).format(DateTime.now()), w),
    ),
  ];
}
