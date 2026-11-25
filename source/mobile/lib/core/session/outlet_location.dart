import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';

import '../../l10n/gen/app_localizations.dart';
import 'session_controller.dart';

/// Tiap aplikasi dibuka dan tiap kasir memilih/pindah outlet: lokasi GPS menimpa koordinat outlet yang sedang aktif
/// (per outlet; server menulis ke outlet milik token). Izin lokasi wajib: sesuai standar Google ditampilkan penjelasan
/// penggunaan dulu (prominent disclosure), baru dialog izin sistem; bila belum diizinkan, diminta lagi pada pembukaan berikutnya.
class OutletLocationGate extends ConsumerStatefulWidget {
  const OutletLocationGate({super.key, required this.child});
  final Widget child;

  @override
  ConsumerState<OutletLocationGate> createState() => _OutletLocationGateState();
}

class _OutletLocationGateState extends ConsumerState<OutletLocationGate> {
  String? _outletId;
  bool _running = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _sync());
  }

  void _sync() {
    final s = ref.read(sessionProvider);
    final id = s is SessionSignedIn ? s.profile.outlet.id : null;
    if (id != null && id != _outletId) {
      _outletId = id;
      _update();
    }
  }

  Future<bool> _ask(String title, String body, String action) async =>
      await showDialog<bool>(
        context: context,
        barrierDismissible: false,
        builder: (c) => AlertDialog(
          icon: const Icon(Icons.location_on_outlined),
          title: Text(title),
          content: Text(body),
          actions: [
            FilledButton(
              onPressed: () => Navigator.pop(c, true),
              child: Text(action),
            ),
          ],
        ),
      ) ??
      false;

  Future<void> _update() async {
    if (_running) return;
    _running = true;
    try {
      final l = AppLocalizations.of(context);
      if (!await Geolocator.isLocationServiceEnabled()) {
        if (mounted &&
            await _ask(l.locTitle, l.locServiceOff, l.locOpenSettings)) {
          await Geolocator.openLocationSettings();
        }
        return;
      }
      var perm = await Geolocator.checkPermission();
      if (perm == LocationPermission.denied) {
        if (!mounted || !await _ask(l.locTitle, l.locBody, l.locAllow)) return;
        perm = await Geolocator.requestPermission();
      }
      if (perm == LocationPermission.deniedForever) {
        if (mounted && await _ask(l.locTitle, l.locDenied, l.locOpenSettings)) {
          await Geolocator.openAppSettings();
        }
        return;
      }
      if (perm == LocationPermission.denied) return;
      final p = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
          timeLimit: Duration(seconds: 20),
        ),
      );
      await ref
          .read(apiClientProvider)
          .dio
          .put<void>(
            '/outlets/current/location',
            data: {'latitude': p.latitude, 'longitude': p.longitude},
          );
    } catch (_) {
      // shortcut: galat GPS/jaringan ditelan tanpa log; dicoba lagi saat dibuka/pindah outlet berikutnya.
    } finally {
      _running = false;
    }
  }

  @override
  Widget build(BuildContext context) {
    ref.listen(sessionProvider, (_, _) => _sync());
    return widget.child;
  }
}
