// Modelo mínimo para GET/PATCH /profile/me (Auth).
// Ver back/auth/internal/handler/http/profile_handler.go:23-30.
// Solo estos 6 campos entrega el backend. No incluye email, rol,
// followers, promedios ni asistencia: no mostrarlos como reales.

class UserProfile {
  UserProfile({
    required this.displayName,
    this.photoUrl,
    this.phone,
    this.institution,
    this.description,
    required this.visibility,
  });

  final String displayName;
  final String? photoUrl;
  final String? phone;
  final String? institution;
  final String? description;
  final String visibility;

  factory UserProfile.fromJson(Map<String, dynamic> j) => UserProfile(
        displayName: (j['display_name'] as String?) ?? '',
        photoUrl: j['photo_url'] as String?,
        phone: j['phone'] as String?,
        institution: j['institution'] as String?,
        description: j['description'] as String?,
        visibility: (j['visibility'] as String?) ?? 'public',
      );

  Map<String, dynamic> toPatchJson() => <String, dynamic>{
        'display_name': displayName,
        if (photoUrl != null) 'photo_url': photoUrl,
        if (phone != null) 'phone': phone,
        if (institution != null) 'institution': institution,
        if (description != null) 'description': description,
        'visibility': visibility,
      };
}

class ProfileApiException implements Exception {
  ProfileApiException(this.message, {this.statusCode, this.code});

  final String message;
  final int? statusCode;
  final String? code;

  @override
  String toString() {
    final c = code ?? 'error';
    final s = statusCode?.toString() ?? '?';
    return '[$c:$s] $message';
  }
}
