import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../models/social_models.dart';
import 'api_config.dart';
import 'session_manager.dart';

// Cliente SOLO para Social (grupos/workspace/todo/sprint/meetings).
// No incluye llamadas de Auth/Perfil (ver profile_service.dart).
// Contratos verificados en:
// - back/social/cmd/server/main.go:182-199
// - back/social/internal/handler/http/group_handler.go, todo_handler.go,
//   sprint_handler.go, meeting_handler.go, view_handler.go
class SocialService {
  SocialService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  static String get baseUrl => socialApiBaseUrl;

  Map<String, String> _headers({bool json = false}) {
    final token = SessionManager.token;
    if (token == null || token.isEmpty) {
      throw SocialApiException(
        'No hay sesión. Inicia sesión para ver tus grupos.',
        code: 'no_session',
      );
    }
    return <String, String>{
      if (json) 'Content-Type': 'application/json',
      'Accept': 'application/json',
      'Authorization': 'Bearer $token',
    };
  }

  SocialApiException _toError(http.Response res) {
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
    if (res.statusCode == 403) {
      message = 'Acceso denegado (403). Debes ser miembro del grupo.';
      code ??= 'forbidden';
    }
    if (res.statusCode == 404) {
      message = 'No encontrado (404). Revisa el ID del grupo.';
      code ??= 'not_found';
    }
    return SocialApiException(message, statusCode: res.statusCode, code: code);
  }

  // GET /me/overview — vista principal global + sidebar.
  Future<Overview> getOverview() async {
    final res = await _client
        .get(Uri.parse('$baseUrl/me/overview'), headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return Overview.fromJson(body);
  }

  // POST /groups {name, description?} -> 201 {group_id}
  Future<String> createGroup({
    required String name,
    String? description,
  }) async {
    final payload = <String, dynamic>{
      'name': name.trim(),
      if (description != null && description.trim().isNotEmpty)
        'description': description.trim(),
    };
    final res = await _client
        .post(
          Uri.parse('$baseUrl/groups'),
          headers: _headers(json: true),
          body: jsonEncode(payload),
        )
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final id = (body['group_id'] as String?) ?? '';
    if (id.isEmpty) {
      throw SocialApiException(
        'El backend no devolvió group_id (201 sin ID).',
        statusCode: 201,
        code: 'missing_group_id',
      );
    }
    return id;
  }

  // GET /groups/:id — detalle real del grupo (GroupView).
  // Ver back/social/internal/handler/http/group_handler.go:119 Get.
  Future<GroupDetail> getGroup(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await _client
        .get(Uri.parse('$baseUrl/groups/$id'), headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return GroupDetail.fromJson(body);
  }

  // GET /groups/:id/workspace — Kanban + Sprint + Meetings + Chat.
  Future<Workspace> getWorkspace(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await _client
        .get(Uri.parse('$baseUrl/groups/$id/workspace'),
            headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return Workspace.fromJson(body);
  }

  // GET /groups/:id/todo?status=&board_id=
  Future<List<TodoTask>> listTodos(String groupId) async {
    final id = groupId.trim();
    final res = await _client
        .get(Uri.parse('$baseUrl/groups/$id/todo'), headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final raw = (body['data'] as List?) ?? const [];
    return raw
        .whereType<Map>()
        .map((e) => TodoTask.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  // POST /groups/:id/todo {title, board_id?, status?, assigned_to?, due_date?}
  Future<TodoTask> createTodo({
    required String groupId,
    required String title,
    String? status,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      if (status != null && status.isNotEmpty) 'status': status,
    };
    final res = await _client
        .post(
          Uri.parse('$baseUrl/groups/${groupId.trim()}/todo'),
          headers: _headers(json: true),
          body: jsonEncode(payload),
        )
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return TodoTask.fromJson(data);
  }

  // GET /groups/:id/sprint-sheet?status=&priority=&sheet_id=
  Future<List<SprintTask>> listSprintTasks(String groupId) async {
    final id = groupId.trim();
    final res = await _client
        .get(Uri.parse('$baseUrl/groups/$id/sprint-sheet'),
            headers: _headers())
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final raw = (body['data'] as List?) ?? const [];
    return raw
        .whereType<Map>()
        .map((e) => SprintTask.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  // POST /groups/:id/sprint-sheet
  // {title required, assigned_to required, sheet_id?, priority?, status?, estimated_hours?}
  // OJO: assigned_to es requerido por el backend (sprint_handler.go:27).
  Future<SprintTask> createSprintTask({
    required String groupId,
    required String title,
    required String assignedTo,
    String? priority,
    String? status,
    double? estimatedHours,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      'assigned_to': assignedTo.trim(),
      if (priority != null && priority.isNotEmpty) 'priority': priority,
      if (status != null && status.isNotEmpty) 'status': status,
      if (estimatedHours != null) 'estimated_hours': estimatedHours,
    };
    final res = await _client
        .post(
          Uri.parse('$baseUrl/groups/${groupId.trim()}/sprint-sheet'),
          headers: _headers(json: true),
          body: jsonEncode(payload),
        )
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return SprintTask.fromJson(data);
  }

  // POST /groups/:id/meetings
  // {title required, description?, scheduled_at RFC3339 required, notify_discord?}
  // -> 201 {meeting_id}. No existe GET /groups/:id/meetings en el backend.
  Future<String> createMeeting({
    required String groupId,
    required String title,
    String? description,
    required DateTime scheduledAtUtc,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      if (description != null && description.trim().isNotEmpty)
        'description': description.trim(),
      'scheduled_at': scheduledAtUtc.toUtc().toIso8601String(),
    };
    debugPrint('[FRONT] POST /groups/$groupId/meetings $payload');
    final res = await _client
        .post(
          Uri.parse('$baseUrl/groups/${groupId.trim()}/meetings'),
          headers: _headers(json: true),
          body: jsonEncode(payload),
        )
        .timeout(const Duration(seconds: 10));
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final id = (body['meeting_id'] as String?) ?? '';
    if (id.isEmpty) {
      throw SocialApiException(
        'El backend no devolvió meeting_id (201 sin ID).',
        statusCode: 201,
        code: 'missing_meeting_id',
      );
    }
    return id;
  }

  void dispose() {
    _client.close();
  }
}
