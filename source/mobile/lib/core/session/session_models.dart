/// Identitas ringkas (pengguna / bisnis / outlet) dari respons auth.
class Identity {
  const Identity({
    required this.id,
    required this.name,
    this.code = '',
    this.email = '',
  });

  final String id;
  final String name;
  final String code;
  final String email;

  factory Identity.fromJson(Map<String, dynamic> j) => Identity(
    id: '${j['id'] ?? ''}',
    name: '${j['name'] ?? ''}',
    code: '${j['code'] ?? ''}',
    email: '${j['email'] ?? ''}',
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'code': code,
    'email': email,
  };
}

/// Izin efektif dari server: `{"*":true}` atau `{"modul":["view","create",...]}`.
/// Hanya untuk menyaring menu/tombol; penegakan tetap di server.
class Permissions {
  const Permissions(this._raw);

  final Map<String, dynamic> _raw;

  static const none = Permissions({});

  factory Permissions.fromJson(Object? json) =>
      json is Map ? Permissions(json.map((k, v) => MapEntry('$k', v))) : none;

  bool can(String module, [String action = 'view']) {
    if (_raw['*'] == true) return true;
    final acts = _raw[module];
    return acts is List && acts.contains(action);
  }

  /// Akun khusus kasir: hanya izin `pos_only` (dan paket kasir). Dipakai untuk langsung masuk ke kasir.
  bool get posOnly => _raw['*'] != true && can('pos_only');

  Map<String, dynamic> toJson() => _raw;
}

/// Profil pengguna yang sedang masuk.
class Profile {
  const Profile({
    required this.user,
    required this.tenant,
    required this.outlet,
    required this.permissions,
    this.emailVerified = true,
  });

  final Identity user;
  final Identity tenant;
  final Identity outlet;
  final Permissions permissions;
  final bool emailVerified;

  factory Profile.fromJson(Map<String, dynamic> j) => Profile(
    user: Identity.fromJson((j['user'] as Map).cast<String, dynamic>()),
    tenant: Identity.fromJson((j['tenant'] as Map).cast<String, dynamic>()),
    outlet: Identity.fromJson((j['outlet'] as Map).cast<String, dynamic>()),
    permissions: Permissions.fromJson(j['permissions']),
    emailVerified: j['email_verified'] != false,
  );

  Map<String, dynamic> toJson() => {
    'user': user.toJson(),
    'tenant': tenant.toJson(),
    'outlet': outlet.toJson(),
    'permissions': permissions.toJson(),
    'email_verified': emailVerified,
  };
}

/// Hasil login / refresh / pindah outlet. [refreshToken] kosong bila server tidak merotasi (mis. pindah outlet).
class AuthSession {
  const AuthSession({
    required this.accessToken,
    required this.expiresIn,
    required this.refreshToken,
    required this.profile,
  });

  final String accessToken;
  final int expiresIn;
  final String refreshToken;
  final Profile profile;

  factory AuthSession.fromJson(Map<String, dynamic> j) => AuthSession(
    accessToken: '${j['access_token']}',
    expiresIn: (j['expires_in'] as num?)?.toInt() ?? 0,
    refreshToken: '${j['refresh_token'] ?? ''}',
    profile: Profile.fromJson(j),
  );
}
