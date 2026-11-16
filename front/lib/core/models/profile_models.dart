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

/// Motivo por el que el texto del buscador no contiene un ID de perfil.
enum PublicIdInputError {
  empty,
  internalUuid,
  webLink,
  unsupportedLink,
  ambiguous,
  invalid,
}

/// Resultado de interpretar el texto pegado en «Buscar perfil».
class PublicIdInput {
  const PublicIdInput.found(String this.publicId) : error = null;
  const PublicIdInput.rejected(PublicIdInputError this.error) : publicId = null;

  final String? publicId;
  final PublicIdInputError? error;

  bool get isValid => publicId != null;
}

const String publicProfileLinkScheme = 'sigma-academy';
const String publicProfileLinkHost = 'perfil';

/// Enlace que se comparte desde «Compartir perfil»:
/// `sigma-academy://perfil/<public_id>`. Devuelve null si [publicId] no es un
/// identificador público válido (nunca se construye con el UUID interno). No
/// es un deep link: solo se copia para pegarlo en «Buscar perfil».
String? publicProfileLink(String publicId) {
  final id = normalizePublicId(publicId);
  if (id == null) return null;
  return '$publicProfileLinkScheme://$publicProfileLinkHost/$id';
}

final RegExp _internalUuidPattern = RegExp(
  r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
);

// Un enlace interno solo cuenta si empieza el texto o va tras un espacio o
// un signo de apertura: así no se acepta un enlace sigma-academy escondido
// dentro de otra URL (por ejemplo en la query de un enlace https).
final RegExp _sigmaLinkPattern = RegExp(
  r'''(?<![^\s(\[<"'«“‘])sigma-academy://\S+''',
  caseSensitive: false,
);
final RegExp _anyLinkPattern = RegExp(r'[a-zA-Z][a-zA-Z0-9+.\-]*://');
const String _trailingPunctuation = '.,;:!?)]}>"\'»”’';

/// Interpreta el texto escrito o pegado en el buscador de perfiles.
///
/// Acepta el identificador público (sin distinguir mayúsculas) o el enlace
/// interno `sigma-academy://perfil/<public_id>`, también dentro de otro texto.
/// Rechaza UUID internos, enlaces http/https, otros esquemas o dominios,
/// enlaces con segmentos, query o fragmento adicionales y textos con enlaces
/// a perfiles distintos. No busca por nombre ni abre enlaces: el usuario pega
/// el ID en la app.
PublicIdInput parsePublicIdInput(String input) {
  final text = input.trim();
  if (text.isEmpty) {
    return const PublicIdInput.rejected(PublicIdInputError.empty);
  }
  final ids = <String>{};
  for (final match in _sigmaLinkPattern.allMatches(text)) {
    final id = _publicIdFromLink(_stripTrailingPunctuation(match.group(0)!));
    if (id == null) {
      return const PublicIdInput.rejected(PublicIdInputError.unsupportedLink);
    }
    ids.add(id);
  }
  if (ids.length > 1) {
    return const PublicIdInput.rejected(PublicIdInputError.ambiguous);
  }
  if (ids.length == 1) return PublicIdInput.found(ids.single);

  if (_anyLinkPattern.hasMatch(text)) {
    final lower = text.toLowerCase();
    final web = lower.contains('http://') || lower.contains('https://');
    return PublicIdInput.rejected(
      web ? PublicIdInputError.webLink : PublicIdInputError.unsupportedLink,
    );
  }
  if (_internalUuidPattern.hasMatch(text)) {
    return const PublicIdInput.rejected(PublicIdInputError.internalUuid);
  }
  final id = normalizePublicId(text);
  return id == null
      ? const PublicIdInput.rejected(PublicIdInputError.invalid)
      : PublicIdInput.found(id);
}

/// Mensaje para mostrar bajo el campo de búsqueda.
String publicIdInputMessage(PublicIdInputError error) {
  switch (error) {
    case PublicIdInputError.empty:
      return 'Escribe o pega un ID de perfil.';
    case PublicIdInputError.internalUuid:
      return 'Eso es un identificador interno. Usa el ID público de 12 '
          'caracteres que aparece en «Mi ID de perfil».';
    case PublicIdInputError.webLink:
      return 'Los enlaces web no son válidos. Pega sigma-academy://perfil/... '
          'o el identificador público.';
    case PublicIdInputError.unsupportedLink:
      return 'El enlace no es válido. Solo se acepta '
          'sigma-academy://perfil/<id>.';
    case PublicIdInputError.ambiguous:
      return 'El texto contiene enlaces a perfiles distintos. Pega solo uno.';
    case PublicIdInputError.invalid:
      return 'No es un ID válido: debe tener 12 caracteres (letras y números, '
          'sin i, l, o ni u).';
  }
}

String _stripTrailingPunctuation(String link) {
  var end = link.length;
  while (end > 0 && _trailingPunctuation.contains(link[end - 1])) {
    end--;
  }
  return link.substring(0, end);
}

String? _publicIdFromLink(String link) {
  final uri = Uri.tryParse(link);
  if (uri == null) return null;
  if (uri.scheme.toLowerCase() != publicProfileLinkScheme) return null;
  if (uri.host.toLowerCase() != publicProfileLinkHost) return null;
  if (uri.userInfo.isNotEmpty || uri.hasPort) return null;
  if (uri.hasQuery || uri.hasFragment) return null;
  if (uri.pathSegments.length != 1) return null;
  return normalizePublicId(uri.pathSegments.single);
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

/// Vista pública de GET /profile/public/:public_id: nombre, institución y
/// foto. Deliberadamente no contiene user_id, email, phone ni la
/// descripción, que es privada.
class PublicProfile {
  const PublicProfile({
    required this.publicId,
    required this.displayName,
    required this.visibility,
    this.institution,
    this.photoDataUrl,
  });

  final String publicId;
  final String displayName;
  final String? institution;
  final String visibility;

  /// Foto subida desde la app (data URL de imagen en base64). Una URL remota
  /// se descarta: abrirla haría que esta app contactara a un servidor ajeno.
  final String? photoDataUrl;

  bool get isPublic => visibility == 'public';

  factory PublicProfile.fromJson(Map<String, dynamic> j) => PublicProfile(
    publicId: (j['public_id'] as String?) ?? '',
    displayName: (j['display_name'] as String?) ?? '',
    institution: j['institution'] as String?,
    visibility: (j['visibility'] as String?) ?? 'public',
    photoDataUrl: imageDataUrlOrNull(j['photo_url'] as String?),
  );
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
