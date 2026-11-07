import 'dart:convert';

import 'package:http/http.dart' as http;

import '../../core/services/api_config.dart';
import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';

/// Nota remota mínima (GET /notes/me) con materia asociada.
class RemoteNote {
  RemoteNote({
    required this.id,
    required this.title,
    required this.content,
    required this.visibility,
    required this.updatedAt,
    this.subjectId,
  });

  final String id;
  final String title;
  final String content;
  final String visibility;
  final DateTime updatedAt;
  final String? subjectId;

  factory RemoteNote.fromJson(Map<String, dynamic> j) {
    DateTime when;
    try {
      when = DateTime.parse(j['updated_at']?.toString() ?? '');
    } catch (_) {
      when = DateTime.fromMillisecondsSinceEpoch(0);
    }
    String? subject = j['subject_id'] as String?;
    if (subject != null && subject.trim().isEmpty) subject = null;
    return RemoteNote(
      id: (j['id'] as String?) ?? '',
      title: (j['title'] as String?) ?? 'Sin título',
      content: (j['content'] as String?) ?? '',
      visibility: (j['visibility'] as String?) ?? 'private',
      updatedAt: when,
      subjectId: subject,
    );
  }
}

/// Mensaje claro para el usuario según el status/código del backend de notas.
/// Centraliza los casos de guardados, likes, copia y búsqueda por ID para que
/// todas las pantallas digan lo mismo.
String notesFriendlyMessage(int? status, String? code, {String? fallback}) {
  switch (code) {
    case 'note_deleted':
      return 'Apunte no disponible: su autor lo eliminó.';
    case 'note_unavailable':
      return 'El contenido del apunte no está disponible en Drive.';
    case 'already_shared':
      return 'Este apunte ya está compartido en este grupo.';
    case 'already_saved':
      return 'Ya tenías este apunte en tus guardados.';
    case 'already_liked':
      return 'Ya le diste like a este apunte.';
    case 'rate_limited':
      return 'Demasiadas copias seguidas. Espera un minuto e inténtalo de nuevo.';
  }
  switch (status) {
    case -1:
      return 'Sin conexión con el servidor de apuntes.';
    case 401:
      return 'Tu sesión venció. Vuelve a iniciar sesión.';
    case 403:
      return 'Conecta o renueva tu Google Drive para continuar.';
    case 404:
      return 'El apunte no existe o no tienes permiso para verlo.';
    case 410:
      return 'Apunte no disponible: su autor lo eliminó.';
    case 429:
      return 'Demasiadas solicitudes. Inténtalo en un momento.';
    case 502:
    case 503:
      return 'Google Drive no respondió. Inténtalo más tarde.';
  }
  return fallback ?? 'Ocurrió un error con el servidor de apuntes.';
}

final RegExp _uuidPattern = RegExp(
  r'[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}',
);

/// Extrae el ID (UUID) de un apunte desde lo que el usuario pegue: el ID solo,
/// con espacios o mayúsculas, o un enlace/URL completo que lo contenga
/// (`https://.../notes/<id>?x=1`, también codificado: `%2D` en vez de `-`).
/// Devuelve el UUID en minúsculas o null si no hay ninguno.
String? extractNoteId(String input) {
  final text = input.trim();
  final direct = _uuidPattern.firstMatch(text);
  if (direct != null) return direct.group(0)!.toLowerCase();
  if (!text.contains('%')) return null;
  String decoded;
  try {
    decoded = Uri.decodeFull(text);
  } catch (_) {
    return null;
  }
  return _uuidPattern.firstMatch(decoded)?.group(0)?.toLowerCase();
}

/// Apunte completo de GET /notes/{id}: metadata, contenido (desde Drive) y
/// contadores/estado del solicitante.
class NoteDetail {
  const NoteDetail({
    required this.id,
    required this.userId,
    required this.title,
    required this.content,
    required this.visibility,
    this.subjectId,
    this.likesCount = 0,
    this.savesCount = 0,
    this.isLiked = false,
    this.isSaved = false,
    this.isOwn = false,
    this.updatedAt,
  });

  final String id;
  final String userId;
  final String title;
  final String content;
  final String visibility;
  final String? subjectId;
  final int likesCount;
  final int savesCount;
  final bool isLiked;
  final bool isSaved;
  final bool isOwn;
  final DateTime? updatedAt;

  bool get isPublic => visibility == 'public';

  factory NoteDetail.fromJson(Map<String, dynamic> j) => NoteDetail(
    id: (j['id'] as String?) ?? '',
    userId: (j['user_id'] as String?) ?? '',
    title: (j['title'] as String?) ?? 'Sin título',
    content: (j['content'] as String?) ?? '',
    visibility: (j['visibility'] as String?) ?? 'private',
    subjectId: j['subject_id'] as String?,
    likesCount: (j['likes_count'] as num?)?.toInt() ?? 0,
    savesCount: (j['saves_count'] as num?)?.toInt() ?? 0,
    isLiked: j['is_liked'] == true,
    isSaved: j['is_saved'] == true,
    isOwn: j['is_own'] == true,
    updatedAt: DateTime.tryParse(j['updated_at']?.toString() ?? ''),
  );

  NoteDetail copyWith({
    int? likesCount,
    int? savesCount,
    bool? isLiked,
    bool? isSaved,
  }) => NoteDetail(
    id: id,
    userId: userId,
    title: title,
    content: content,
    visibility: visibility,
    subjectId: subjectId,
    likesCount: likesCount ?? this.likesCount,
    savesCount: savesCount ?? this.savesCount,
    isLiked: isLiked ?? this.isLiked,
    isSaved: isSaved ?? this.isSaved,
    isOwn: isOwn,
    updatedAt: updatedAt,
  );
}

/// Contadores exactos de un apunte y estado del usuario (GET /notes/{id}/stats).
class NoteInteractions {
  const NoteInteractions({
    required this.noteId,
    this.likesCount = 0,
    this.savesCount = 0,
    this.isLiked = false,
    this.isSaved = false,
  });

  final String noteId;
  final int likesCount;
  final int savesCount;
  final bool isLiked;
  final bool isSaved;

  factory NoteInteractions.fromJson(Map<String, dynamic> j, String noteId) =>
      NoteInteractions(
        noteId: (j['note_id'] as String?) ?? noteId,
        likesCount: (j['likes_count'] as num?)?.toInt() ?? 0,
        savesCount: (j['saves_count'] as num?)?.toInt() ?? 0,
        isLiked: j['is_liked'] == true,
        isSaved: j['is_saved'] == true,
      );

  NoteInteractions copyWith({
    int? likesCount,
    int? savesCount,
    bool? isLiked,
    bool? isSaved,
  }) => NoteInteractions(
    noteId: noteId,
    likesCount: likesCount ?? this.likesCount,
    savesCount: savesCount ?? this.savesCount,
    isLiked: isLiked ?? this.isLiked,
    isSaved: isSaved ?? this.isSaved,
  );
}

/// Página de un listado con cursor (`{items, next_cursor}`).
class NotesPage<T> {
  const NotesPage(this.items, this.nextCursor);

  final List<T> items;
  final String nextCursor;

  bool get hasMore => nextCursor.isNotEmpty;
}

/// Apunte público de otro usuario (GET /notes/public).
class PublicNoteSummary {
  const PublicNoteSummary({
    required this.id,
    required this.userId,
    required this.title,
    required this.interactions,
    this.subjectId,
    this.updatedAt,
  });

  final String id;
  final String userId;
  final String title;
  final String? subjectId;
  final DateTime? updatedAt;
  final NoteInteractions interactions;

  factory PublicNoteSummary.fromJson(Map<String, dynamic> j) {
    final id = (j['id'] as String?) ?? '';
    return PublicNoteSummary(
      id: id,
      userId: (j['user_id'] as String?) ?? '',
      title: (j['title'] as String?) ?? 'Sin título',
      subjectId: j['subject_id'] as String?,
      updatedAt: DateTime.tryParse(j['updated_at']?.toString() ?? ''),
      interactions: NoteInteractions.fromJson(j, id),
    );
  }
}

/// Filtros de GET /notes/saved. `scope`: all | others | own;
/// `availability`: all | available | unavailable.
class SavedNotesFilter {
  const SavedNotesFilter({
    this.query = '',
    this.scope = 'all',
    this.availability = 'all',
    this.subjectId,
    this.authorId,
  });

  final String query;
  final String scope;
  final String availability;
  final String? subjectId;
  final String? authorId;

  Map<String, String> toQuery() => <String, String>{
    if (query.trim().isNotEmpty) 'q': query.trim(),
    if (scope != 'all') 'scope': scope,
    if (availability != 'all') 'availability': availability,
    if ((subjectId ?? '').isNotEmpty) 'subject_id': subjectId!,
    if ((authorId ?? '').isNotEmpty) 'author_id': authorId!,
  };

  SavedNotesFilter copyWith({
    String? query,
    String? scope,
    String? availability,
  }) => SavedNotesFilter(
    query: query ?? this.query,
    scope: scope ?? this.scope,
    availability: availability ?? this.availability,
    subjectId: subjectId,
    authorId: authorId,
  );
}

/// Elemento de GET /notes/saved. Si [isAvailable] es false, el apunte original
/// fue eliminado por su autor (`deleted`) o ya no es accesible (`no_access`):
/// solo se permite quitarlo de guardados.
class SavedNoteItem {
  const SavedNoteItem({
    required this.savedId,
    required this.noteId,
    required this.title,
    required this.isAvailable,
    required this.interactions,
    this.unavailableReason,
    this.authorId,
    this.subjectId,
    this.isOwn = false,
    this.savedAt,
    this.updatedAt,
    this.deletedAt,
  });

  final String savedId;
  final String noteId;
  final String title;
  final bool isAvailable;
  final String? unavailableReason;
  final String? authorId;
  final String? subjectId;
  final bool isOwn;
  final DateTime? savedAt;
  final DateTime? updatedAt;
  final DateTime? deletedAt;
  final NoteInteractions interactions;

  bool get isDeleted => unavailableReason == 'deleted';

  factory SavedNoteItem.fromJson(Map<String, dynamic> j) {
    final noteId = (j['note_id'] as String?) ?? '';
    return SavedNoteItem(
      savedId: (j['saved_id'] as String?) ?? '',
      noteId: noteId,
      title: (j['title'] as String?) ?? 'Apunte',
      isAvailable: j['is_available'] == true,
      unavailableReason: j['unavailable_reason'] as String?,
      authorId: j['user_id'] as String?,
      subjectId: j['subject_id'] as String?,
      isOwn: j['is_own'] == true,
      savedAt: DateTime.tryParse(j['saved_at']?.toString() ?? ''),
      updatedAt: DateTime.tryParse(j['updated_at']?.toString() ?? ''),
      deletedAt: DateTime.tryParse(j['deleted_at']?.toString() ?? ''),
      interactions: NoteInteractions.fromJson(j, noteId),
    );
  }
}

/// Resultado de POST /notes/{id}/copy: el clon propio del usuario.
class NoteCopyResult {
  const NoteCopyResult({
    required this.noteId,
    required this.forkedFromNoteId,
    required this.title,
    this.version = 1,
  });

  final String noteId;
  final String forkedFromNoteId;
  final String title;
  final int version;

  factory NoteCopyResult.fromJson(Map<String, dynamic> j, String sourceId) =>
      NoteCopyResult(
        noteId: (j['note_id'] as String?) ?? '',
        forkedFromNoteId: (j['forked_from_note_id'] as String?) ?? sourceId,
        title: (j['title'] as String?) ?? '',
        version: (j['version'] as num?)?.toInt() ?? 1,
      );
}

class NotesApiException implements Exception {
  NotesApiException(this.message, {this.statusCode, this.code});

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

/// Cliente mínimo del backend de notas para academic (UX materiales).
/// Misma base que el módulo (api_config) y mismo reintento 401 que el
/// resto del front (AuthedHttp -> login al morir la sesión).
class NotesApi {
  NotesApi({http.Client? client, String? baseUrl})
    : _client = client ?? http.Client(),
      _baseUrl = baseUrl ?? notesApiBaseUrl;

  final http.Client _client;
  final String _baseUrl;

  void dispose() => _client.close();

  Map<String, String> _headers({bool json = false}) {
    final token = SessionManager.token;
    if (token == null || token.isEmpty) {
      throw NotesApiException(
        'No hay sesión. Inicia sesión para ver tus apuntes.',
        code: 'no_session',
      );
    }
    return <String, String>{
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      'Authorization': 'Bearer $token',
    };
  }

  NotesApiException _toError(http.Response res, String fallback) {
    String message = fallback;
    String? code;
    try {
      if (res.body.isNotEmpty) {
        final body = jsonDecode(utf8.decode(res.bodyBytes));
        if (body is Map) {
          final err = body['error'];
          if (err is Map) {
            code = err['code']?.toString();
            message = err['message']?.toString() ?? err.toString();
          } else if (body['message'] is String) {
            message = body['message'] as String;
          }
        }
      }
    } catch (_) {}
    if (res.statusCode == 401) {
      message = 'No autorizado (401). Revisa tu sesión.';
      code ??= 'unauthorized';
    }
    if (res.statusCode != 401 &&
        (code == 'note_deleted' ||
            code == 'already_shared' ||
            code == 'rate_limited' ||
            res.statusCode == 410)) {
      message = notesFriendlyMessage(res.statusCode, code, fallback: message);
    }
    return NotesApiException(message, statusCode: res.statusCode, code: code);
  }

  /// GET /notes/me del usuario con sesión.
  Future<List<RemoteNote>> listMyNotes({int limit = 50}) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$_baseUrl/notes/me?limit=$limit'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) {
      throw _toError(res, 'No se pudieron cargar los apuntes.');
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final items = body is Map ? body['notes'] : null;
    if (items is! List) {
      throw NotesApiException('Respuesta inesperada del servidor.');
    }
    return items
        .whereType<Map>()
        .map((e) => RemoteNote.fromJson(Map<String, dynamic>.from(e)))
        .toList();
  }

  /// Apuntes de una materia (filtro `subject_id === materia`, lado cliente).
  Future<List<RemoteNote>> listBySubject(String subjectId) async {
    final all = await listMyNotes();
    final want = subjectId.trim();
    final out = all.where((n) => n.subjectId == want).toList()
      ..sort((a, b) => b.updatedAt.compareTo(a.updatedAt));
    return out;
  }

  // POST /notes {title, content, visibility} -> 201 {note_id}.
  Future<String> createNote({
    required String title,
    required String content,
    required String visibility,
  }) async {
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/notes'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{
              'title': title.trim(),
              'content': content.trim(),
              'visibility': visibility.trim(),
            }),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201) throw _toError(res, 'No se pudo crear la nota.');
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    final id = (body['note_id'] as String?) ?? (body['id'] as String?) ?? '';
    if (id.isEmpty) {
      throw NotesApiException(
        'El backend no devolvió note_id.',
        statusCode: res.statusCode,
        code: 'missing_note_id',
      );
    }
    return id;
  }

  // POST /notes/:note_id/share {group_id, access_mode} -> 201 {shared_note_id}.
  Future<String> shareNote({
    required String noteId,
    required String groupId,
    required String accessMode,
  }) async {
    final nid = noteId.trim();
    final gid = groupId.trim();
    if (nid.isEmpty || gid.isEmpty) {
      throw NotesApiException(
        'noteId y groupId requeridos.',
        code: 'invalid_args',
      );
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$_baseUrl/notes/$nid/share'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{
              'group_id': gid,
              'access_mode': accessMode.trim(),
            }),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201) {
      throw _toError(res, 'No se pudo compartir la nota.');
    }
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    final id =
        (body['shared_note_id'] as String?) ?? (body['id'] as String?) ?? '';
    if (id.isEmpty) {
      throw NotesApiException(
        'El backend no devolvió shared_note_id.',
        statusCode: res.statusCode,
        code: 'missing_shared_note_id',
      );
    }
    return id;
  }

  /// POST /notes/:id/copy -> 201 {note_id, forked_from_note_id, title, ...}.
  /// Clona el apunte (guardado o público) al Drive y la BD del usuario actual.
  /// [idempotencyKey] hace seguro el reintento: el backend devuelve el mismo
  /// clon en vez de duplicarlo.
  Future<NoteCopyResult> copyNote(
    String noteId, {
    String? idempotencyKey,
  }) async {
    final nid = noteId.trim();
    if (nid.isEmpty) {
      throw NotesApiException('noteId requerido.', code: 'invalid_args');
    }
    final headers = _headers();
    if (idempotencyKey != null && idempotencyKey.isNotEmpty) {
      headers['X-Idempotency-Key'] = idempotencyKey;
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(Uri.parse('$_baseUrl/notes/$nid/copy'), headers: headers)
          .timeout(const Duration(seconds: 30)),
    );
    if (res.statusCode != 201) {
      final err = _toError(res, 'No se pudo copiar el apunte.');
      throw NotesApiException(
        notesFriendlyMessage(res.statusCode, err.code, fallback: err.message),
        statusCode: res.statusCode,
        code: err.code,
      );
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map) {
      throw NotesApiException('Respuesta inesperada del servidor.');
    }
    final result = NoteCopyResult.fromJson(
      Map<String, dynamic>.from(body),
      nid,
    );
    if (result.noteId.isEmpty) {
      throw NotesApiException(
        'El backend no devolvió note_id.',
        statusCode: res.statusCode,
        code: 'missing_note_id',
      );
    }
    return result;
  }

  /// GET /notes/:id — búsqueda directa por ID exacto. Respeta permisos en el
  /// backend: 404 si no existe o no tienes acceso (indistinguibles), 410 si
  /// el autor lo eliminó, 404 note_unavailable si Drive no lo entrega.
  Future<NoteDetail> getNote(String noteIdOrLink) async {
    final nid = extractNoteId(noteIdOrLink);
    if (nid == null) {
      throw NotesApiException(
        'Eso no parece un ID de apunte válido.',
        code: 'invalid_id',
      );
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$_baseUrl/notes/$nid'), headers: _headers())
          .timeout(const Duration(seconds: 20)),
    );
    if (res.statusCode != 200) {
      final err = _toError(res, 'No se pudo abrir el apunte.');
      throw NotesApiException(
        notesFriendlyMessage(res.statusCode, err.code, fallback: err.message),
        statusCode: res.statusCode,
        code: err.code,
      );
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map) {
      throw NotesApiException('Respuesta inesperada del servidor.');
    }
    return NoteDetail.fromJson(Map<String, dynamic>.from(body));
  }

  NotesApiException _friendly(http.Response res, String fallback) {
    final err = _toError(res, fallback);
    return NotesApiException(
      notesFriendlyMessage(res.statusCode, err.code, fallback: err.message),
      statusCode: res.statusCode,
      code: err.code,
    );
  }

  Future<http.Response> _send(
    String method,
    String path, {
    Map<String, String>? query,
  }) {
    final uri = Uri.parse(
      '$_baseUrl$path',
    ).replace(queryParameters: (query == null || query.isEmpty) ? null : query);
    return AuthedHttp.run(() {
      final headers = _headers();
      final Future<http.Response> req;
      switch (method) {
        case 'POST':
          req = _client.post(uri, headers: headers);
          break;
        case 'DELETE':
          req = _client.delete(uri, headers: headers);
          break;
        default:
          req = _client.get(uri, headers: headers);
      }
      return req.timeout(const Duration(seconds: 15));
    });
  }

  Map<String, dynamic> _decodeMap(http.Response res) {
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    if (body is! Map) {
      throw NotesApiException('Respuesta inesperada del servidor.');
    }
    return Map<String, dynamic>.from(body);
  }

  /// GET /notes/:id/stats — contadores exactos sin descargar el contenido.
  Future<NoteInteractions> stats(String noteId) async {
    final res = await _send('GET', '/notes/$noteId/stats');
    if (res.statusCode != 200) {
      throw _friendly(res, 'No se pudieron leer los contadores.');
    }
    return NoteInteractions.fromJson(_decodeMap(res), noteId);
  }

  /// Da (liked=true) o quita (false) el like. Idempotente desde el cliente:
  /// el 409 already_liked y el 204 de quitar se resuelven releyendo los
  /// contadores, así que el resultado siempre es el estado real del servidor.
  Future<NoteInteractions> setLiked(String noteId, bool liked) async {
    final res = await _send(liked ? 'POST' : 'DELETE', '/notes/$noteId/like');
    if (liked && res.statusCode == 201 && res.body.isNotEmpty) {
      return NoteInteractions.fromJson(_decodeMap(res), noteId);
    }
    final ok = liked
        ? (res.statusCode == 201 || res.statusCode == 409)
        : res.statusCode == 204;
    if (!ok) throw _friendly(res, 'No se pudo actualizar el like.');
    return stats(noteId);
  }

  /// Guarda (saved=true) o quita de guardados (false). Idempotente igual que
  /// [setLiked]. Quitar funciona aunque el original ya no exista; en ese
  /// caso no hay contadores que releer y se devuelve null.
  Future<NoteInteractions?> setSaved(String noteId, bool saved) async {
    final res = await _send(saved ? 'POST' : 'DELETE', '/notes/$noteId/save');
    if (saved && res.statusCode == 201 && res.body.isNotEmpty) {
      return NoteInteractions.fromJson(_decodeMap(res), noteId);
    }
    final ok = saved
        ? (res.statusCode == 201 || res.statusCode == 409)
        : res.statusCode == 204;
    if (!ok) throw _friendly(res, 'No se pudo actualizar el guardado.');
    try {
      return await stats(noteId);
    } on NotesApiException {
      if (!saved) return null;
      rethrow;
    }
  }

  /// GET /notes/public — apuntes públicos de otros usuarios.
  Future<NotesPage<PublicNoteSummary>> listPublic({
    String query = '',
    String? subjectId,
    String? cursor,
    int limit = 20,
  }) async {
    final res = await _send(
      'GET',
      '/notes/public',
      query: <String, String>{
        if (query.trim().isNotEmpty) 'q': query.trim(),
        if ((subjectId ?? '').isNotEmpty) 'subject_id': subjectId!,
        if ((cursor ?? '').isNotEmpty) 'cursor': cursor!,
        'limit': '$limit',
      },
    );
    if (res.statusCode != 200) {
      throw _friendly(res, 'No se pudieron cargar los apuntes públicos.');
    }
    final body = _decodeMap(res);
    final items = (body['items'] as List? ?? const [])
        .whereType<Map>()
        .map((e) => PublicNoteSummary.fromJson(Map<String, dynamic>.from(e)))
        .toList();
    return NotesPage(items, (body['next_cursor'] as String?) ?? '');
  }

  /// GET /notes/saved — guardados del usuario con filtros, incluidos los
  /// "no disponibles" (is_available=false).
  Future<NotesPage<SavedNoteItem>> listSaved({
    SavedNotesFilter filter = const SavedNotesFilter(),
    String? cursor,
    int limit = 20,
  }) async {
    final res = await _send(
      'GET',
      '/notes/saved',
      query: <String, String>{
        ...filter.toQuery(),
        if ((cursor ?? '').isNotEmpty) 'cursor': cursor!,
        'limit': '$limit',
      },
    );
    if (res.statusCode != 200) {
      throw _friendly(res, 'No se pudieron cargar tus guardados.');
    }
    final body = _decodeMap(res);
    final items = (body['items'] as List? ?? const [])
        .whereType<Map>()
        .map((e) => SavedNoteItem.fromJson(Map<String, dynamic>.from(e)))
        .toList();
    return NotesPage(items, (body['next_cursor'] as String?) ?? '');
  }
}
