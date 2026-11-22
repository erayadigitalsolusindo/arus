import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../l10n/gen/app_localizations.dart';
import '../api/api_error.dart';
import '../session/session_controller.dart';
import '../theme/app_theme.dart';
import 'qris_toko.dart';

/// Pasang/buka EraPay Listener (penangkap notifikasi pembayaran QRISGate). Muncul bersama kartu QRIS toko, yaitu
/// hanya bila jalur pembayaran toko ke QRISGate. Tautan setup dari EraPay mengisi app Listener tanpa ketik ulang;
/// bila app belum terpasang, dashboard EraPay dibuka (masuk otomatis) untuk mengunduh APK-nya.
class ListenerTile extends ConsumerWidget {
  const ListenerTile({super.key});

  Future<String> _post(WidgetRef ref, String path, String key) async {
    final r = await ref
        .read(apiClientProvider)
        .dio
        .post<Map<String, dynamic>>(path);
    return '${r.data?[key] ?? ''}';
  }

  Future<void> _open(BuildContext context, WidgetRef ref) async {
    final l = AppLocalizations.of(context);
    final messenger = ScaffoldMessenger.of(context);
    try {
      final link = await _post(ref, '/ecosystem/erapay/listener-link', 'link');
      if (link.isNotEmpty &&
          await launchUrl(
            Uri.parse(link),
            mode: LaunchMode.externalApplication,
          )) {
        return;
      }
      final dash = await _post(ref, '/ecosystem/erapay/sso', 'url');
      messenger.showSnackBar(SnackBar(content: Text(l.listenerNotInstalled)));
      await launchUrl(Uri.parse(dash), mode: LaunchMode.externalApplication);
    } on DioException catch (e) {
      messenger.showSnackBar(
        SnackBar(content: Text(ApiError.fromDio(e).message(l))),
      );
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final q = ref.watch(qrisTokoProvider).value;
    if (q == null || !q.active) return const SizedBox.shrink();
    final l = AppLocalizations.of(context);
    final pal = context.pal;
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: Material(
        color: pal.primarySoft,
        borderRadius: BorderRadius.circular(12),
        child: ListTile(
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
          leading: Icon(
            Icons.notifications_active_outlined,
            color: pal.primary,
          ),
          title: Text(
            l.listenerTitle,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
          subtitle: Text(
            l.listenerSubtitle,
            style: TextStyle(fontSize: 12, color: pal.textTertiary),
          ),
          onTap: () => _open(context, ref),
        ),
      ),
    );
  }
}
