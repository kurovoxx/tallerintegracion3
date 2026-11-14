/// Identificador público del perfil: 12 caracteres Crockford Base32 en
/// minúsculas (60 bits). Excluye i, l, o y u para evitar confusiones al
/// transcribir. Es lo único que se comparte; nunca el UUID interno.
final RegExp _publicIdPattern = RegExp(r'^[0-9a-hj-km-np-tv-z]{12}$');

bool isValidPublicId(String value) => _publicIdPattern.hasMatch(value);

/// Normaliza el ID que el usuario escribe o pega (recorta espacios y pasa a
/// minúsculas) y devuelve null si no cumple estrictamente el formato.
///
/// No convierte `l`/`o` a `1`/`0`: un carácter excluido invalida la entrada.
/// Tampoco acepta UUID, nombres, correos, teléfonos ni enlaces.
String? normalizePublicId(String input) {
  final candidate = input.trim().toLowerCase();
  if (candidate.isEmpty) return null;
  return isValidPublicId(candidate) ? candidate : null;
}

// Modelo para GET/PATCH /profile/me (Auth).
// El backend entrega display_name, photo_url, phone, institution,
// description, visibility, public_id y email (backward-compatible).
class UserProfile {
  UserProfile({
    required this.displayName,
    this.publicId = '',
    this.photoUrl,
    this.phone,
    this.institution,
    this.description,
    required this.visibility,
    this.email,
  });

  final String displayName;

  /// Identificador público que se comparte; nunca es el UUID interno.
  final String publicId;
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
      publicId: (j['public_id'] as String?) ?? '',
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

/// Vista pública de GET /profile/public/:public_id: solo nombre e
/// institución. Deliberadamente no contiene user_id, email, phone, foto ni la
/// descripción, que es privada.
class PublicProfile {
  const PublicProfile({
    required this.publicId,
    required this.displayName,
    required this.visibility,
    this.institution,
  });

  final String publicId;
  final String displayName;
  final String? institution;
  final String visibility;

  bool get isPublic => visibility == 'public';

  factory PublicProfile.fromJson(Map<String, dynamic> j) => PublicProfile(
    publicId: (j['public_id'] as String?) ?? '',
    displayName: (j['display_name'] as String?) ?? '',
    institution: j['institution'] as String?,
    visibility: (j['visibility'] as String?) ?? 'public',
  );
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
