import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/session/session_controller.dart';
import '../../core/api/api_error.dart';
import '../../l10n/gen/app_localizations.dart';

/// Chat HalloArus (asisten AI) bertema terminal IT. Memakai POST /ai/chat yang sama dengan web; riwayat
/// dikirim utuh tiap giliran. Usulan aksi tulis dari AI (kas kecil, dll.) belum ditampilkan di mobile: lakukan di web.
Future<void> showHalloArusPage(BuildContext context) => Navigator.of(
  context,
).push<void>(MaterialPageRoute(builder: (_) => const HalloArusPage()));

const _bg = Color(0xFF070B14);
const _panel = Color(0xFF0E1626);
const _cyan = Color(0xFF22D3EE);
const _violet = Color(0xFFA78BFA);
const _mono = 'monospace';

class _Msg {
  _Msg(this.user, this.text, {this.error = false});
  final bool user;
  final String text;
  final bool error;
}

class HalloArusPage extends ConsumerStatefulWidget {
  const HalloArusPage({super.key});

  @override
  ConsumerState<HalloArusPage> createState() => _HalloArusPageState();
}

class _HalloArusPageState extends ConsumerState<HalloArusPage> {
  final _msgs = <_Msg>[];
  final _input = TextEditingController();
  final _scroll = ScrollController();
  String _conversationId = '';
  bool _busy = false;

  @override
  void dispose() {
    _input.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _toBottom() => WidgetsBinding.instance.addPostFrameCallback((_) {
    if (_scroll.hasClients) {
      _scroll.animateTo(
        _scroll.position.maxScrollExtent,
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOut,
      );
    }
  });

  Future<void> _send() async {
    final text = _input.text.trim();
    if (text.isEmpty || _busy) return;
    _input.clear();
    setState(() {
      _msgs.add(_Msg(true, text));
      _busy = true;
    });
    _toBottom();
    try {
      final r = await ref
          .read(apiClientProvider)
          .dio
          .post<Map<String, dynamic>>(
            '/ai/chat',
            data: {
              'messages': [
                for (final m in _msgs.where((m) => !m.error))
                  {'role': m.user ? 'user' : 'assistant', 'content': m.text},
              ],
              if (_conversationId.isNotEmpty) 'conversation_id': _conversationId,
            },
            // Satu pertanyaan bisa butuh beberapa putaran tool di server (sampai 3 menit).
            options: Options(receiveTimeout: const Duration(minutes: 3)),
          );
      _conversationId = '${r.data!['conversation_id'] ?? _conversationId}';
      _msgs.add(_Msg(false, '${r.data!['reply'] ?? ''}'));
    } on DioException catch (e) {
      final err = ApiError.fromDio(e);
      _msgs.add(
        _Msg(
          false,
          err.serverMessage.isNotEmpty ? err.serverMessage : err.code,
          error: true,
        ),
      );
    }
    if (!mounted) return;
    setState(() => _busy = false);
    _toBottom();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    return Scaffold(
      backgroundColor: _bg,
      appBar: AppBar(
        backgroundColor: _bg,
        foregroundColor: Colors.white,
        elevation: 0,
        titleSpacing: 0,
        title: Row(
          children: [
            const _Orb(size: 28),
            const SizedBox(width: 10),
            Text(
              'HalloArus',
              style: const TextStyle(
                fontFamily: _mono,
                fontWeight: FontWeight.w800,
                letterSpacing: 0.5,
              ),
            ),
            const SizedBox(width: 8),
            const Text(
              'AI',
              style: TextStyle(
                fontFamily: _mono,
                color: _cyan,
                fontWeight: FontWeight.w800,
                fontSize: 12,
              ),
            ),
          ],
        ),
      ),
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: _msgs.isEmpty
                  ? _Empty(l.halloArusEmpty)
                  : ListView.builder(
                      controller: _scroll,
                      padding: const EdgeInsets.fromLTRB(14, 8, 14, 8),
                      itemCount: _msgs.length + (_busy ? 1 : 0),
                      itemBuilder: (_, i) => i == _msgs.length
                          ? _Bubble(
                              user: false,
                              child: Text(
                                '${l.halloArusThinking}…',
                                style: const TextStyle(
                                  fontFamily: _mono,
                                  color: _cyan,
                                ),
                              ),
                            )
                          : _Bubble(
                              user: _msgs[i].user,
                              child: SelectableText(
                                _msgs[i].text,
                                style: TextStyle(
                                  fontFamily: _mono,
                                  fontSize: 14,
                                  height: 1.45,
                                  color: _msgs[i].error
                                      ? const Color(0xFFFCA5A5)
                                      : Colors.white,
                                ),
                              ),
                            ),
                    ),
            ),
            Container(
              padding: const EdgeInsets.fromLTRB(12, 8, 8, 10),
              decoration: const BoxDecoration(
                color: _panel,
                border: Border(top: BorderSide(color: Color(0xFF1E2A44))),
              ),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  const Padding(
                    padding: EdgeInsets.only(bottom: 14),
                    child: Text(
                      '>',
                      style: TextStyle(
                        fontFamily: _mono,
                        color: _cyan,
                        fontSize: 18,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: TextField(
                      controller: _input,
                      minLines: 1,
                      maxLines: 4,
                      textInputAction: TextInputAction.send,
                      onSubmitted: (_) => _send(),
                      style: const TextStyle(
                        fontFamily: _mono,
                        color: Colors.white,
                      ),
                      cursorColor: _cyan,
                      decoration: InputDecoration(
                        hintText: l.halloArusHint,
                        hintStyle: const TextStyle(
                          fontFamily: _mono,
                          color: Color(0xFF5B6B8C),
                        ),
                        border: InputBorder.none,
                        filled: false,
                      ),
                    ),
                  ),
                  IconButton(
                    onPressed: _busy ? null : _send,
                    icon: const Icon(Icons.send_rounded),
                    color: _cyan,
                    disabledColor: const Color(0xFF334155),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Bola gradien cyan→ungu penanda AI.
class _Orb extends StatelessWidget {
  const _Orb({required this.size});
  final double size;

  @override
  Widget build(BuildContext context) => Container(
    width: size,
    height: size,
    decoration: BoxDecoration(
      shape: BoxShape.circle,
      gradient: const LinearGradient(
        colors: [_cyan, _violet],
        begin: Alignment.topLeft,
        end: Alignment.bottomRight,
      ),
      boxShadow: [
        BoxShadow(color: _cyan.withValues(alpha: 0.5), blurRadius: size / 2),
      ],
    ),
    child: Icon(Icons.auto_awesome, size: size * 0.55, color: Colors.white),
  );
}

class _Empty extends StatelessWidget {
  const _Empty(this.text);
  final String text;

  @override
  Widget build(BuildContext context) => Center(
    child: Padding(
      padding: const EdgeInsets.all(32),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const _Orb(size: 72),
          const SizedBox(height: 20),
          Text(
            text,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontFamily: _mono,
              color: Color(0xFF94A3B8),
              height: 1.5,
            ),
          ),
        ],
      ),
    ),
  );
}

class _Bubble extends StatelessWidget {
  const _Bubble({required this.user, required this.child});
  final bool user;
  final Widget child;

  @override
  Widget build(BuildContext context) => Align(
    alignment: user ? Alignment.centerRight : Alignment.centerLeft,
    child: Container(
      margin: const EdgeInsets.symmetric(vertical: 5),
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
      constraints: BoxConstraints(
        maxWidth: MediaQuery.sizeOf(context).width * 0.86,
      ),
      decoration: BoxDecoration(
        color: user ? const Color(0xFF16254A) : _panel,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: user ? _violet.withValues(alpha: 0.6) : _cyan.withValues(alpha: 0.35),
        ),
      ),
      child: child,
    ),
  );
}
