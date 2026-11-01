// Modelo para GET/PATCH /profile/me (Auth).
// El backend entrega display_name, photo_url, phone, institution,
// description, visibility y email (identity.users, backward-compatible).
class UserProfile {
  UserProfile({
    required this.displayName,
    this.photoUrl,
    this.phone,
    this.institution,
    this.description,
    required this.visibility,
    this.email,
  });

  final String displayName;
  final String? photoUrl;
  final String? phone;
  final String? institution;
  final String? description;
  final String visibility;
  final String? email;

  factory UserProfile.fromJson(Map<String, dynamic> j) {
    String? mail = j['email'] as String?;
    if (mail != null && mail.trim().isEmpty) mail = null;
    return UserProfile(
      displayName: (j['display_name'] as String?) ?? '',
      photoUrl: j['photo_url'] as String?,
      phone: j['phone'] as String?,
      institution: j['institution'] as String?,
      description: j['description'] as String?,
      visibility: (j['visibility'] as String?) ?? 'public',
      email: mail,
    );
  }

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
