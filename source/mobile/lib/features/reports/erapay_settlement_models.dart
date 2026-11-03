import 'package:decimal/decimal.dart';

import '../../core/money/money.dart';

/// Satu tagihan QRIS EraPay: uang menurut ARUS vs uang menurut EraPay, status, dan ID transaksi.
class SettlementRow {
  const SettlementRow({
    required this.chargeId,
    required this.paymentId,
    required this.gateway,
    required this.status,
    required this.state,
    required this.amount,
    required this.arusAmount,
    required this.eraAmount,
    required this.createdAt,
    required this.outlet,
    required this.cashier,
    required this.docNo,
  });

  final String chargeId;

  /// ID transaksi di EraPay (kosong bila tagihan belum sampai ke EraPay).
  final String paymentId;
  final String gateway;

  /// Status di EraPay: succeeded | pending | failed | expired | cancelled | unknown.
  final String status;

  /// Keadaan menurut server: matched | waiting | orphan | refund | unpaid | closed.
  final String state;
  final Decimal amount;
  final Decimal arusAmount;
  final Decimal eraAmount;
  final DateTime createdAt;
  final String outlet;
  final String cashier;
  final String docNo;

  bool get needsAttention =>
      state == 'orphan' || state == 'refund' || state == 'unpaid';

  factory SettlementRow.fromJson(Map<String, dynamic> j) => SettlementRow(
    chargeId: '${j['charge_id']}',
    paymentId: '${j['payment_id'] ?? ''}',
    gateway: '${j['gateway'] ?? ''}',
    status: '${j['status'] ?? ''}',
    state: '${j['state'] ?? ''}',
    amount: dec(j['amount']),
    arusAmount: dec(j['arus_amount']),
    eraAmount: dec(j['era_amount']),
    createdAt:
        DateTime.tryParse('${j['created_at']}')?.toLocal() ?? DateTime.now(),
    outlet: '${j['outlet'] ?? ''}',
    cashier: '${j['cashier'] ?? ''}',
    docNo: '${j['doc_no'] ?? ''}',
  );
}

class EraSettlementReport {
  const EraSettlementReport({
    required this.configured,
    required this.refreshed,
    required this.truncated,
    required this.arus,
    required this.era,
    required this.diff,
    required this.count,
    required this.matched,
    required this.waiting,
    required this.attention,
    required this.rows,
  });

  final bool configured;
  final int refreshed;
  final bool truncated;
  final Decimal arus;
  final Decimal era;

  /// Uang EraPay dikurangi uang ARUS; bukan nol = ada yang perlu dicek.
  final Decimal diff;
  final int count;
  final int matched;
  final int waiting;
  final int attention;
  final List<SettlementRow> rows;

  factory EraSettlementReport.fromJson(Map<String, dynamic> j) {
    final t = (j['totals'] as Map?)?.cast<String, dynamic>() ?? const {};
    int n(String k) => (t[k] as num?)?.toInt() ?? 0;
    return EraSettlementReport(
      configured: j['configured'] == true,
      refreshed: (j['refreshed'] as num?)?.toInt() ?? 0,
      truncated: j['truncated'] == true,
      arus: dec(t['arus']),
      era: dec(t['era']),
      diff: dec(t['diff']),
      count: n('count'),
      matched: n('matched'),
      waiting: n('waiting'),
      attention: n('attention'),
      rows: (j['data'] as List? ?? const [])
          .map(
            (e) => SettlementRow.fromJson((e as Map).cast<String, dynamic>()),
          )
          .toList(),
    );
  }
}
