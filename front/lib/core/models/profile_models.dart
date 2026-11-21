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

/// Motivo por el que el texto del buscador no es un código de perfil.
enum PublicIdInputError { empty, internalUuid, link, invalid }

/// Resultado de interpretar el texto escrito en «Buscar perfil».
class PublicIdInput {
  const PublicIdInput.found(String this.publicId) : error = null;
  const PublicIdInput.rejected(PublicIdInputError this.error) : publicId = null;

  final String? publicId;
  final PublicIdInputError? error;

  bool get isValid => publicId != null;
}

final RegExp _internalUuidPattern = RegExp(
  r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
);
final RegExp _anyLinkPattern = RegExp(r'[a-zA-Z][a-zA-Z0-9+.\-]*://');

/// Interpreta el texto escrito o pegado en el buscador de perfiles.
///
/// Acepta únicamente el código público del perfil (12 caracteres, sin
/// distinguir mayúsculas ni espacios alrededor). «Compartir perfil» copia
/// solo ese código: ya no se generan ni se aceptan enlaces de ningún tipo
/// (tampoco los antiguos `sigma-academy://perfil/...`). Rechaza también los
/// identificadores internos, nombres, correos y teléfonos.
PublicIdInput parsePublicIdInput(String input) {
  final text = input.trim();
  if (text.isEmpty) {
    return const PublicIdInput.rejected(PublicIdInputError.empty);
  }
  if (_anyLinkPattern.hasMatch(text)) {
    return const PublicIdInput.rejected(PublicIdInputError.link);
  }
  if (_internalUuidPattern.hasMatch(text)) {
    return const PublicIdInput.rejected(PublicIdInputError.internalUuid);
  }
  final id = normalizePublicId(text);
  return id == null
      ? const PublicIdInput.rejected(PublicIdInputError.invalid)
      : PublicIdInput.found(id);
}

/// Mensaje para mostrar bajo el campo de búsqueda, sin términos técnicos.
String publicIdInputMessage(PublicIdInputError error) {
  switch (error) {
    case PublicIdInputError.empty:
      return 'Ingresa el código del perfil que deseas buscar.';
    case PublicIdInputError.internalUuid:
      return 'Ese no es un código de perfil. Pide a la persona el código de '
          '12 caracteres que copia con «Compartir perfil».';
    case PublicIdInputError.link:
      return 'Ingresa solo el código del perfil, no un enlace.';
    case PublicIdInputError.invalid:
      return 'El código no es válido: debe tener 12 letras y números '
          '(sin i, l, o ni u).';
  }
}

// Modelo para GET/PATCH /profile/me (Auth).
// El backend entrega display_name, photo_url, phone, institution,
// description, visibility, public_id y email (backward-compatible).
class UserProfile {
  UserProfile({
    required this.displayName,
    this.publicId = '',
    this.photoUrl,
    this.photoThumbUrl,
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

  /// Miniatura recortada para listados/sidebar; si falta, usar photoUrl.
  final String? photoThumbUrl;
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
      photoThumbUrl: j['photo_thumb_url'] as String?,
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
    if (photoThumbUrl != null) 'photo_thumb_url': photoThumbUrl,
    if (phone != null) 'phone': phone,
    if (institution != null) 'institution': institution,
    if (description != null) 'description': description,
    'visibility': visibility,
  };
}

/// Vista de GET /profile/public/:public_id. Perfil público: nombre, foto,
/// descripción, correo, teléfono e institución. Perfil privado: solo nombre y
/// foto (Auth no envía el resto). Nunca contiene el identificador interno.
class PublicProfile {
  const PublicProfile({
    required this.publicId,
    required this.displayName,
    required this.visibility,
    this.institution,
    this.description,
    this.email,
    this.phone,
    this.photoDataUrl,
  });

  final String publicId;
  final String displayName;
  final String? institution;
  final String? description;
  final String? email;
  final String? phone;
  final String visibility;

  /// Foto subida desde la app (data URL de imagen en base64). Una URL remota
  /// se descarta: abrirla haría que esta app contactara a un servidor ajeno.
  final String? photoDataUrl;

  bool get isPublic => visibility == 'public';

  /// Un perfil privado solo muestra nombre y foto, aunque una respuesta
  /// inesperada trajera más campos: el cliente también los descarta.
  factory PublicProfile.fromJson(Map<String, dynamic> j) {
    final visibility = (j['visibility'] as String?) == 'public'
        ? 'public'
        : 'private';
    final isPublic = visibility == 'public';
    String? field(String key) {
      if (!isPublic) return null;
      final v = (j[key] as String?)?.trim() ?? '';
      return v.isEmpty ? null : v;
    }

    return PublicProfile(
      publicId: (j['public_id'] as String?) ?? '',
      displayName: (j['display_name'] as String?) ?? '',
      visibility: visibility,
      institution: field('institution'),
      description: field('description'),
      email: field('email'),
      phone: field('phone'),
      photoDataUrl: imageDataUrlOrNull(j['photo_url'] as String?),
    );
  }
}

/// Devuelve [value] solo si es una data URL de imagen en base64.
String? imageDataUrlOrNull(String? value) {
  final v = value?.trim() ?? '';
  if (!v.startsWith('data:image/') || !v.contains(';base64,')) return null;
  return v;
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

/// Estado de "seguir" respecto a un perfil público.
class FollowStatus {
  const FollowStatus({
    required this.publicId,
    required this.following,
  });

  final String publicId;
  final bool following;
}

/// Usuario seguido (viene de GET /profile/following/:public_id).
/// Datos mínimos públicos para mostrar en la lista de seguidos.
class FollowedUser {
  const FollowedUser({
    required this.userId,
    required this.publicId,
    required this.displayName,
    this.photoThumbUrl,
    required this.followedAt,
  });

  final String userId;
  final String publicId;
  final String displayName;
  final String? photoThumbUrl;
  final DateTime followedAt;

  factory FollowedUser.fromJson(Map<String, dynamic> j) {
    DateTime? followed;
    final raw = j['followed_at'];
    if (raw is String) followed = DateTime.tryParse(raw);
    return FollowedUser(
      userId: (j['user_id'] as String?) ?? '',
      publicId: (j['public_id'] as String?) ?? '',
      displayName: (j['display_name'] as String?) ?? 'Sin nombre',
      photoThumbUrl: j['photo_thumb_url'] as String?,
      followedAt: followed ?? DateTime.now(),
    );
  }
}
