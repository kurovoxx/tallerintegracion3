import 'dart:convert';

import 'package:http/http.dart' as http;

import 'api_config.dart';
import 'authed_client.dart';
import 'session_manager.dart';

/// Error human-readable de la API de notas compartidas (Notes HTTP).
class NotesApiException implements Exception {
  NotesApiException(this.message, {this.statusCode, this.code});

  final String message;
  final int? statusCode;
  final String? code;

  @override
  String toString() => '[${code ?? 'error'}:${statusCode ?? '?'}] $message';
}

/// Nota compartida con un grupo tal como la devuelve
/// `GET /groups/{id}/notes` (subset de la nota + vínculo, sin contenido).
class SharedGroupNote {
  const SharedGroupNote({
    required this.noteId,
    required this.title,
    required this.visibility,
    required this.likesCount,
    required this.version,
    this.ownerUserId,
  });

  final String noteId;
  final String title;
  final String visibility;
  final int likesCount;
  final int version;
  final String? ownerUserId;

  factory SharedGroupNote.fromJson(Map<String, dynamic> json) {
    return SharedGroupNote(
      noteId: (json['id'] ?? '').toString(),
      title: (json['title'] ?? 'Sin título').toString(),
      visibility: (json['visibility'] ?? 'private').toString(),
      likesCount: (json['likes_count'] as num?)?.toInt() ?? 0,
      version: (json['version'] as num?)?.toInt() ?? 1,
      ownerUserId: json['user_id']?.toString(),
    );
  }
}

/// Nota propia (GET /notes/me) para el selector al compartir.
class MyNoteSummary {
  const MyNoteSummary({
    required this.noteId,
    required this.title,
    required this.visibility,
  });

  final String noteId;
  final String title;
  final String visibility;

  factory MyNoteSummary.fromJson(Map<String, dynamic> json) {
    return MyNoteSummary(
      noteId: (json['id'] ?? '').toString(),
      title: (json['title'] ?? 'Sin título').toString(),
      visibility: (json['visibility'] ?? 'private').toString(),
    );
  }
}

/// Resultado de abrir una nota compartida (contenido descargado de Drive).
class SharedNoteContent {
  const SharedNoteContent({required this.title, required this.content});

  final String title;
  final String content;
}

/// Cliente de la API pública de Notes (HTTP) para notas compartidas en grupos.
///
/// Contratos (ver back/notes/cmd/server/main.go y note_handler.go):
/// - GET    {notes}/groups/{id}/notes        -> 200 {notes: [...], next_cursor}
/// - GET    {notes}/notes/me                  -> 200 {notes: [...], next_cursor}
/// - POST   {notes}/notes/{id}/share          -> 201 {shared_note_id}
/// - DELETE {notes}/notes/shared/{sharedId}   -> 204
/// - GET    {notes}/notes/{id}                -> 200 {title, content?, ...}
class NotesSharingService {
  NotesSharingService({http.Client? client})
    : _client = client ?? http.Client();

  final http.Client _client;

  static String get baseUrl => notesApiBaseUrl;

  /// Modos válidos del backend (back/notes/internal/utils/validator.go).
  static const String accessModeLink = 'link';
  static const String accessModeRestricted = 'restricted';

  Map<String, String> _headers({bool json = false}) {
    final token = SessionManager.token;
    if (token == null || token.isEmpty) {
      throw NotesApiException(
        'No hay sesión. Inicia sesión para compartir notas.',
        code: 'no_session',
      );
    }
    return <String, String>{
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      'Authorization': 'Bearer $token',
    };
  }

  NotesApiException _toError(http.Response res) {
    String message = 'Error HTTP ${res.statusCode}';
    String? code;
    try {
      if (res.body.isNotEmpty) {
        final body = jsonDecode(utf8.decode(res.bodyBytes));
        if (body is Map) {
          final err = body['error'];
          if (err is Map) {
            code = err['code']?.toString();
            message = err['message']?.toString() ?? err.toString();
          } else if (err is String) {
            message = err;
          }
        }
      }
    } catch (_) {}
    switch (res.statusCode) {
      case 401:
        message = 'Tu sesión venció. Vuelve a iniciar sesión.';
        code ??= 'unauthorized';
        break;
      case 403:
        message = 'No tienes permiso (403). Debes ser miembro del grupo.';
        code ??= 'forbidden';
        break;
      case 404:
        message = 'No encontrado (404).';
        code ??= 'not_found';
        break;
      case 409:
        message = 'Conflicto (409).';
        code ??= 'conflict';
        break;
    }
    return NotesApiException(message, statusCode: res.statusCode, code: code);
  }

  /// Lista las notas compartidas con [groupId] (más recientes primero).
  /// Sigue la paginación por cursor hasta agotarla (tope de seguridad).
  Future<List<SharedGroupNote>> listGroupNotes(
    String groupId, {
    int pageSize = 50,
    int maxPages = 10,
  }) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw NotesApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final out = <SharedGroupNote>[];
    String cursor = '';
    for (var page = 0; page < maxPages; page++) {
      final query = <String, String>{'limit': '$pageSize'};
      if (cursor.isNotEmpty) query['cursor'] = cursor;
      final uri = Uri.parse(
        '$baseUrl/groups/$id/notes',
      ).replace(queryParameters: query);
      final res = await AuthedHttp.run(
        () => _client.get(uri, headers: _headers()).timeout(
          const Duration(seconds: 12),
        ),
      );
      if (res.statusCode != 200) throw _toError(res);
      final body = jsonDecode(utf8.decode(res.bodyBytes));
      final raw = (body is Map ? body['notes'] : null) ?? const [];
      for (final e in (raw as List).whereType<Map>()) {
        out.add(SharedGroupNote.fromJson(Map<String, dynamic>.from(e)));
      }
      final next = (body is Map ? body['next_cursor']?.toString() : '') ?? '';
      if (next.isEmpty) break;
      cursor = next;
    }
    return out;
  }

  /// Lista mis notas (para elegir cuál compartir).
  Future<List<MyNoteSummary>> listMyNotes({int limit = 50}) async {
    final uri = Uri.parse(
      '$baseUrl/notes/me',
    ).replace(queryParameters: {'limit': '$limit'});
    final res = await AuthedHttp.run(
      () => _client.get(uri, headers: _headers()).timeout(
        const Duration(seconds: 12),
      ),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final raw = (body is Map ? body['notes'] : null) ?? const [];
    return (raw as List)
        .whereType<Map>()
        .map((e) => MyNoteSummary.fromJson(Map<String, dynamic>.from(e)))
        .toList();
  }

  /// Comparte [noteId] con [groupId] en [accessMode] (link|restricted).
  /// Devuelve el `shared_note_id` necesario para dejar de compartir.
  Future<String> shareNote({
    required String noteId,
    required String groupId,
    required String accessMode,
  }) async {
    final nid = noteId.trim();
    final gid = groupId.trim();
    if (nid.isEmpty || gid.isEmpty) {
      throw NotesApiException('noteId/groupId vacíos.', code: 'invalid_args');
    }
    if (accessMode != accessModeLink && accessMode != accessModeRestricted) {
      throw NotesApiException(
        'Modo inválido: usa link o restricted.',
        code: 'invalid_access_mode',
      );
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/notes/$nid/share'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{
              'group_id': gid,
              'access_mode': accessMode,
            }),
          )
          .timeout(const Duration(seconds: 15)),
    );
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final sharedId = (body is Map ? body['shared_note_id']?.toString() : '') ?? '';
    if (sharedId.isEmpty) {
      throw NotesApiException(
        'El backend no devolvió shared_note_id.',
        statusCode: 201,
        code: 'missing_shared_note_id',
      );
    }
    return sharedId;
  }

  /// Deja de compartir un vínculo (DELETE /notes/shared/{sharedNoteId}).
  Future<void> unshare(String sharedNoteId) async {
    final sid = sharedNoteId.trim();
    if (sid.isEmpty) {
      throw NotesApiException('sharedNoteId vacío.', code: 'invalid_args');
    }
    final res = await AuthedHttp.run(
      () => _client.delete(
        Uri.parse('$baseUrl/notes/shared/$sid'),
        headers: _headers(),
      ).timeout(const Duration(seconds: 12)),
    );
    // 204 idempotente; 404 = ya no estaba (se trata como éxito).
    if (res.statusCode != 204 && res.statusCode != 404) throw _toError(res);
  }

  /// Descarga el contenido de una nota compartida (GET /notes/{id}).
  /// 404 zero-knowledge = sin acceso o inexistente.
  Future<SharedNoteContent> getNoteContent(String noteId) async {
    final nid = noteId.trim();
    if (nid.isEmpty) {
      throw NotesApiException('noteId vacío.', code: 'invalid_args');
    }
    final res = await AuthedHttp.run(
      () => _client.get(
        Uri.parse('$baseUrl/notes/$nid'),
        headers: _headers(),
      ).timeout(const Duration(seconds: 15)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final map = body is Map ? Map<String, dynamic>.from(body) : const {};
    return SharedNoteContent(
      title: (map['title'] ?? 'Nota').toString(),
      content: (map['content'] ?? '').toString(),
    );
  }

  void dispose() => _client.close();
}
