import 'dart:async';
import 'dart:math';

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'session_controller.dart';

/// Presence "online": selama login dan aplikasi di layar kirim heartbeat 35–55 dtk; ke background/ditutup → offline.
class PresenceBeacon extends ConsumerStatefulWidget {
  const PresenceBeacon({super.key, required this.child});
  final Widget child;

  @override
  ConsumerState<PresenceBeacon> createState() => _PresenceBeaconState();
}

class _PresenceBeaconState extends ConsumerState<PresenceBeacon>
    with WidgetsBindingObserver {
  Timer? _timer;
  final _rnd = Random();

  bool get _signedIn => ref.read(sessionProvider) is SessionSignedIn;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _schedule();
  }

  void _schedule() {
    _timer?.cancel();
    _timer = Timer(Duration(seconds: 35 + _rnd.nextInt(20)), () {
      if (_signedIn) unawaited(ref.read(apiClientProvider).ping());
      _schedule();
    });
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState s) {
    if (!_signedIn) return;
    final api = ref.read(apiClientProvider);
    if (s == AppLifecycleState.resumed) {
      unawaited(api.ping());
      _schedule();
    } else if (s == AppLifecycleState.paused ||
        s == AppLifecycleState.detached) {
      _timer?.cancel();
      unawaited(api.offline());
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
