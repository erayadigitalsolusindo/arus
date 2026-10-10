import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';

import 'package:arus_mobile/core/api/api_client.dart';
import 'package:arus_mobile/core/session/token_store.dart';

/// Adapter HTTP palsu: jawaban ditentukan per "METHOD path".
class FakeAdapter implements HttpClientAdapter {
  FakeAdapter(this.handler);

  final ResponseBody Function(RequestOptions o) handler;
  final List<RequestOptions> calls = [];

  @override
  Future<ResponseBody> fetch(
    RequestOptions o,
    Stream<Uint8List>? body,
    Future<void>? cancel,
  ) async {
    calls.add(o);
    return handler(o);
  }

  @override
  void close({bool force = false}) {}
}

ResponseBody json(int status, Map<String, dynamic> body) =>
    ResponseBody.fromString(
      jsonEncode(body),
      status,
      headers: {
        Headers.contentTypeHeader: ['application/json'],
      },
    );

Map<String, dynamic> sessionBody({
  String refresh = 'r1',
  String access = 'a1',
}) => {
  'access_token': access,
  'expires_in': 900,
  if (refresh.isNotEmpty) 'refresh_token': refresh,
  'permissions': {
    'sales_orders': ['view', 'create'],
  },
  'email_verified': true,
  'user': {'id': 'u1', 'name': 'Budi', 'email': 'budi@toko.test'},
  'tenant': {'id': 't1', 'name': 'Toko Maju', 'code': 'maju'},
  'outlet': {'id': 'o1', 'name': 'Pusat', 'code': 'MAIN'},
};

ApiClient clientWith(
  FakeAdapter raw, {
  FakeAdapter? authed,
  TokenStore? tokens,
  void Function()? onExpired,
}) {
  return ApiClient(
    tokens: tokens ?? MemoryTokenStore(),
    onSessionExpired: onExpired ?? () {},
    raw: Dio(BaseOptions(baseUrl: 'http://x'))..httpClientAdapter = raw,
    authed: Dio(BaseOptions(baseUrl: 'http://x'))
      ..httpClientAdapter = authed ?? raw,
  );
}
