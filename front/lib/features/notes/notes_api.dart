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
    return NotesApiException(
      message,
      statusCode: res.statusCode,
      code: code,
    );
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
      throw NotesApiException('El backend no devolvió note_id.', statusCode: res.statusCode, code: 'missing_note_id');
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
      throw NotesApiException('noteId y groupId requeridos.', code: 'invalid_args');
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
    if (res.statusCode != 201) throw _toError(res, 'No se pudo compartir la nota.');
    final body = jsonDecode(res.body) as Map<String, dynamic>;
    final id = (body['shared_note_id'] as String?) ?? (body['id'] as String?) ?? '';
    if (id.isEmpty) {
      throw NotesApiException(
        'El backend no devolvió shared_note_id.',
        statusCode: res.statusCode,
        code: 'missing_shared_note_id',
      );
    }
    return id;
  }
}
