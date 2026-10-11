import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:print_bluetooth_thermal/print_bluetooth_thermal.dart';

import '../../l10n/gen/app_localizations.dart';
import '../theme/app_theme.dart';
import 'printer_service.dart';
import 'receipt_layout.dart';

/// Pengaturan printer Bluetooth (per perangkat): pilih printer yang sudah dipasangkan, lebar kertas, potong, cetak otomatis.
Future<void> showPrinterSheet(BuildContext context) =>
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      useSafeArea: true,
      showDragHandle: true,
      builder: (_) => const _PrinterSheet(),
    );

class _PrinterSheet extends ConsumerStatefulWidget {
  const _PrinterSheet();

  @override
  ConsumerState<_PrinterSheet> createState() => _PrinterSheetState();
}

class _PrinterSheetState extends ConsumerState<_PrinterSheet> {
  List<BluetoothInfo>? _paired;
  Object? _error;
  bool _loading = false;
  bool _testing = false;
  String? _testMsg;
  bool _testOk = false;

  @override
  void initState() {
    super.initState();
    _loadPaired();
  }

  Future<void> _loadPaired() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await ref.read(receiptPrinterProvider).paired();
      if (mounted) setState(() => _paired = list);
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _update(PrinterSettings s) =>
      ref.read(printerSettingsProvider.notifier).save(s);

  Future<void> _test(PrinterSettings s) async {
    final l = AppLocalizations.of(context);
    setState(() {
      _testing = true;
      _testMsg = null;
    });
    try {
      await ref.read(receiptPrinterProvider).printTest(l, settings: s);
      if (mounted) {
        setState(() {
          _testOk = true;
          _testMsg = l.printerTestSent;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _testOk = false;
          _testMsg = printErrorMessage(l, e);
        });
      }
    } finally {
      if (mounted) setState(() => _testing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    final s = ref.watch(printerSettingsProvider).value ?? const PrinterSettings();
    final paired = _paired;

    return SafeArea(
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 0, 20, 20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    l.printerTitle,
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w800,
                    ),
                  ),
                ),
                IconButton(
                  tooltip: l.printerRefresh,
                  onPressed: _loading ? null : _loadPaired,
                  icon: const Icon(Icons.refresh),
                ),
              ],
            ),
            Text(
              l.printerHint,
              style: TextStyle(color: pal.textTertiary, fontSize: 12.5),
            ),
            const SizedBox(height: 12),
            if (_loading)
              const Padding(
                padding: EdgeInsets.all(16),
                child: Center(child: CircularProgressIndicator()),
              )
            else if (_error != null)
              Text(
                printErrorMessage(l, _error!),
                style: TextStyle(color: pal.danger),
              )
            else if (paired != null && paired.isEmpty)
              Text(l.printerNonePaired, style: TextStyle(color: pal.danger))
            else if (paired != null)
              RadioGroup<String>(
                groupValue: s.address,
                onChanged: (mac) {
                  if (mac == null) return;
                  final d = paired.firstWhere((p) => p.macAdress == mac);
                  _update(s.copyWith(address: d.macAdress, name: d.name));
                },
                child: Column(
                  children: [
                    for (final d in paired)
                      RadioListTile<String>(
                        contentPadding: EdgeInsets.zero,
                        dense: true,
                        value: d.macAdress,
                        title: Text(d.name),
                        subtitle: Text(d.macAdress),
                      ),
                  ],
                ),
              ),
            if (s.hasPrinter && (paired == null || _error != null))
              ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.print),
                title: Text(s.name.isEmpty ? s.address : s.name),
                subtitle: Text(s.address),
              ),
            const Divider(height: 24),
            Text(
              l.printerPaper,
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            SegmentedButton<Paper>(
              segments: [
                ButtonSegment(value: Paper.mm58, label: Text(l.printerPaper58)),
                ButtonSegment(value: Paper.mm80, label: Text(l.printerPaper80)),
              ],
              selected: {s.paper},
              onSelectionChanged: (v) => _update(s.copyWith(paper: v.first)),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(l.printerCut),
              value: s.cut,
              onChanged: (v) => _update(s.copyWith(cut: v)),
            ),
            SwitchListTile(
              contentPadding: EdgeInsets.zero,
              title: Text(l.printerAuto),
              value: s.auto,
              onChanged: (v) => _update(s.copyWith(auto: v)),
            ),
            const SizedBox(height: 8),
            FilledButton.icon(
              onPressed: s.hasPrinter && !_testing ? () => _test(s) : null,
              icon: _testing
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.print_outlined),
              label: Text(l.printerTest),
            ),
            if (_testMsg != null)
              Padding(
                padding: const EdgeInsets.only(top: 8),
                child: Text(
                  _testMsg!,
                  style: TextStyle(color: _testOk ? pal.success : pal.danger),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// Tombol "Cetak struk" / "Cetak ulang" untuk satu nota. Belum ada printer → membuka pengaturan printer lebih dulu.
class PrintReceiptButton extends ConsumerStatefulWidget {
  const PrintReceiptButton({
    super.key,
    required this.saleId,
    this.reprint = false,
  });

  final String saleId;
  final bool reprint;

  @override
  ConsumerState<PrintReceiptButton> createState() => _PrintReceiptButtonState();
}

class _PrintReceiptButtonState extends ConsumerState<PrintReceiptButton> {
  bool _busy = false;
  String? _error;

  Future<void> _print() async {
    final l = AppLocalizations.of(context);
    var s = await ref.read(printerSettingsProvider.future);
    if (!s.hasPrinter) {
      if (!mounted) return;
      await showPrinterSheet(context);
      s = await ref.read(printerSettingsProvider.future);
      if (!s.hasPrinter) return;
    }
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await ref
          .read(receiptPrinterProvider)
          .printSale(widget.saleId, l, reprint: widget.reprint);
    } catch (e) {
      if (mounted) setState(() => _error = printErrorMessage(l, e));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        OutlinedButton.icon(
          onPressed: _busy ? null : _print,
          icon: _busy
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.print_outlined),
          label: Text(
            _busy
                ? l.printPrinting
                : (widget.reprint ? l.printReprint : l.printReceipt),
          ),
        ),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              _error!,
              textAlign: TextAlign.center,
              style: TextStyle(color: context.pal.danger, fontSize: 12.5),
            ),
          ),
      ],
    );
  }
}
