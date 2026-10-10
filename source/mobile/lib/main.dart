import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';
import 'core/config/server_url.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final savedUrl = await loadSavedServerUrl();
  runApp(
    ProviderScope(
      overrides: [initialServerUrlProvider.overrideWithValue(savedUrl)],
      child: const ArusApp(),
    ),
  );
}
