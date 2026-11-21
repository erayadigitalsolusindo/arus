import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../l10n/gen/app_localizations.dart';
import '../api/api_error.dart';
import '../session/session_controller.dart';
import '../theme/app_theme.dart';

// Skema yang didaftarkan app EraPay Listener; dipakai juga untuk mendeteksi apakah app itu sudah terpasang.
final _probe = Uri.parse('erapaylistener://setup');

/// Ambil tautan setup dari EraPay (lewat backend ARUS) lalu buka di app Listener; belum terpasang = ajakan memasang.
Future<void> setupErapayListener(BuildContext context, WidgetRef ref) async {
  final l = AppLocalizations.of(context);
  final messenger = ScaffoldMessenger.of(context);
  final dio = ref.read(apiClientProvider).dio;
  Future<String> post(String path, String key) async {
    final r = await dio.post<Map<String, dynamic>>(path);
    return '${r.data?[key] ?? ''}';
  }

  try {
    if (!await canLaunchUrl(_probe)) {
      if (!context.mounted) return;
      await _askInstall(context, l, () async {
        final url = await post('/ecosystem/erapay/sso', 'url');
        await launchUrl(Uri.parse(url), mode: LaunchMode.externalApplication);
      }, messenger);
      return;
    }
    final link = await post('/ecosystem/erapay/listener-setup', 'link');
    if (!await launchUrl(
      Uri.parse(link),
      mode: LaunchMode.externalApplication,
    )) {
      throw ApiError(code: 'UNKNOWN');
    }
  } on DioException catch (e) {
    messenger.showSnackBar(
      SnackBar(content: Text(ApiError.fromDio(e).message(l))),
    );
  } on ApiError catch (e) {
    messenger.showSnackBar(SnackBar(content: Text(e.message(l))));
  }
}

Future<void> _askInstall(
  BuildContext context,
  AppLocalizations l,
  Future<void> Function() openDashboard,
  ScaffoldMessengerState messenger,
) {
  return showDialog<void>(
    context: context,
    builder: (ctx) => AlertDialog(
      icon: Icon(Icons.download_for_offline_outlined, color: ctx.pal.primary),
      title: Text(l.listenerInstallTitle),
      content: Text(l.listenerInstallBody),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(ctx),
          child: Text(l.listenerClose),
        ),
        FilledButton(
          onPressed: () async {
            Navigator.pop(ctx);
            try {
              await openDashboard();
            } on DioException catch (e) {
              messenger.showSnackBar(
                SnackBar(content: Text(ApiError.fromDio(e).message(l))),
              );
            }
          },
          child: Text(l.listenerInstallOpen),
        ),
      ],
    ),
  );
}
