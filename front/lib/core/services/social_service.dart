import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import '../models/social_models.dart';
import 'api_config.dart';
import 'authed_client.dart';
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

  /// Versión global de grupos: se incrementa tras create/join/leave para que
  /// GroupsScreen y el sidebar (MainShell) se refresquen sin botón manual.
  static final ValueNotifier<int> groupsChanged = ValueNotifier<int>(0);

  static void notifyGroupsChanged() => groupsChanged.value++;

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
    if (res.statusCode == 503) {
      message = 'Servicio no disponible (503). Intenta más tarde.';
      code ??= 'unavailable';
    }
    return SocialApiException(message, statusCode: res.statusCode, code: code);
  }

  // GET /me/overview — vista principal global + sidebar.
  Future<Overview> getOverview() async {
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/me/overview'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
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
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
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
    notifyGroupsChanged();
    return id;
  }

  // GET /groups/:id — detalle real del grupo (GroupView).
  // Ver back/social/internal/handler/http/group_handler.go:119 Get.
  Future<GroupDetail> getGroup(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/groups/$id'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return GroupDetail.fromJson(body);
  }

  // POST /groups/:id/invite/regenerate -> 200 {new_invite_token}.
  // Solo admin. Invalida enlaces anteriores.
  Future<String> regenerateInvite(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/$id/invite/regenerate'),
            headers: _headers(json: true),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final token = (body['new_invite_token'] as String?) ?? '';
    if (token.isEmpty) {
      throw SocialApiException(
        'El backend no devolvió invitación.',
        statusCode: 200,
        code: 'missing_invite_token',
      );
    }
    return token;
  }

  // GET /groups/:id/workspace — Kanban + Sprint + Meetings + Chat.
  Future<Workspace> getWorkspace(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/groups/$id/workspace'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return Workspace.fromJson(body);
  }

  // GET /groups/:id/members — lista directa de integrantes.
  // Ver back/social/internal/handler/http/group_handler.go:185 ListMembers.
  Future<List<GroupMember>> listMembers(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/groups/$id/members'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes));
    final raw = (body as List?) ?? const [];
    return raw
        .whereType<Map>()
        .map((e) => GroupMember.fromJson(Map<String, dynamic>.from(e as Map)))
        .toList();
  }

  // GET /groups/:id/stream-token -> 200 {token, channel_id}.
  // Ver back/social/internal/handler/http/stream_handler.go:26 Token.
  // 401 sesión inválida, 403 no miembro, 404 grupo inexistente,
  // 503 Stream no configurado. El token nunca se muestra ni se registra.
  Future<StreamToken> getStreamToken(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$baseUrl/groups/$id/stream-token'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final token = StreamToken.fromJson(body);
    if (token.token.isEmpty) {
      throw SocialApiException(
        'El backend no devolvió token (200 sin token).',
        statusCode: 200,
        code: 'missing_stream_token',
      );
    }
    return token;
  }

  // GET /groups/:id/todo?status=&board_id=
  Future<List<TodoTask>> listTodos(String groupId) async {
    final id = groupId.trim();
    final res = await AuthedHttp.run(
      () => _client
          .get(Uri.parse('$baseUrl/groups/$id/todo'), headers: _headers())
          .timeout(const Duration(seconds: 10)),
    );
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
    String? assignedTo,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      if (status != null && status.isNotEmpty) 'status': status,
      if (assignedTo != null && assignedTo.trim().isNotEmpty)
        'assigned_to': assignedTo.trim(),
    };
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/${groupId.trim()}/todo'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return TodoTask.fromJson(data);
  }

  // PATCH /groups/:id/todo/:taskId — campos parciales.
  // Ver back/social/internal/handler/http/todo_handler.go:171 UpdateTodo.
  // Responde 200 {message, data}.
  Future<TodoTask> updateTodo({
    required String groupId,
    required String taskId,
    String? title,
    String? status,
    String? assignedTo,
  }) async {
    final payload = <String, dynamic>{
      if (title != null) 'title': title.trim(),
      if (status != null && status.isNotEmpty) 'status': status,
      if (assignedTo != null) 'assigned_to': assignedTo.trim(),
    };
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse(
              '$baseUrl/groups/${groupId.trim()}/todo/${taskId.trim()}',
            ),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return TodoTask.fromJson(data);
  }

  // DELETE /groups/:id/todo/:taskId -> 200 {message, data}.
  Future<void> deleteTodo({
    required String groupId,
    required String taskId,
  }) async {
    final res = await AuthedHttp.run(
      () => _client
          .delete(
            Uri.parse(
              '$baseUrl/groups/${groupId.trim()}/todo/${taskId.trim()}',
            ),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
  }

  Future<List<SprintSheetInfo>> listSprintSheets(String groupId) async {
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$baseUrl/groups/$groupId/sprint-sheets'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final data = (jsonDecode(res.body) as Map<String, dynamic>)['data'] as List;
    return data
        .map((e) => SprintSheetInfo.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<SprintSheetInfo> saveSprintSheet({
    required String groupId,
    String? sheetId,
    required String name,
    required String start,
    required String end,
  }) async {
    final url = Uri.parse(
      '$baseUrl/groups/$groupId/sprint-sheets${sheetId == null ? '' : '/$sheetId'}',
    );
    final body = jsonEncode({
      'name': name.trim(),
      'period_start': start,
      'period_end': end,
    });
    final res = await AuthedHttp.run(
      () =>
          (sheetId == null
                  ? _client.post(url, headers: _headers(json: true), body: body)
                  : _client.patch(
                      url,
                      headers: _headers(json: true),
                      body: body,
                    ))
              .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200 && res.statusCode != 201) throw _toError(res);
    return SprintSheetInfo.fromJson(
      (jsonDecode(res.body) as Map<String, dynamic>)['data']
          as Map<String, dynamic>,
    );
  }

  // GET /groups/:id/sprint-sheet?status=&priority=&sheet_id=
  Future<List<SprintTask>> listSprintTasks(String groupId) async {
    final id = groupId.trim();
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$baseUrl/groups/$id/sprint-sheet'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
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
    String? sheetId,
    required String title,
    required String assignedTo,
    String? priority,
    String? status,
    double? estimatedHours,
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      'sheet_id': ?sheetId,
      'assigned_to': assignedTo.trim(),
      if (priority != null && priority.isNotEmpty) 'priority': priority,
      if (status != null && status.isNotEmpty) 'status': status,
      if (estimatedHours != null) 'estimated_hours': estimatedHours,
    };
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/${groupId.trim()}/sprint-sheet'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return SprintTask.fromJson(data);
  }

  // PATCH /groups/:id/sprint-sheet/:taskId — campos parciales.
  // Ver back/social/internal/handler/http/sprint_handler.go:172.
  // Responde 200 {message, data}.
  Future<SprintTask> updateSprintTask({
    required String groupId,
    required String taskId,
    String? title,
    String? assignedTo,
    String? priority,
    String? status,
    double? estimatedHours,
  }) async {
    final payload = <String, dynamic>{
      if (title != null) 'title': title.trim(),
      if (assignedTo != null) 'assigned_to': assignedTo.trim(),
      if (priority != null && priority.isNotEmpty) 'priority': priority,
      if (status != null && status.isNotEmpty) 'status': status,
      if (estimatedHours != null) 'estimated_hours': estimatedHours,
    };
    final res = await AuthedHttp.run(
      () => _client
          .patch(
            Uri.parse(
              '$baseUrl/groups/${groupId.trim()}/sprint-sheet/${taskId.trim()}',
            ),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return SprintTask.fromJson(data);
  }

  // DELETE /groups/:id/sprint-sheet/:taskId -> 200 {message, data}.
  Future<void> deleteSprintTask({
    required String groupId,
    required String taskId,
  }) async {
    final res = await AuthedHttp.run(
      () => _client
          .delete(
            Uri.parse(
              '$baseUrl/groups/${groupId.trim()}/sprint-sheet/${taskId.trim()}',
            ),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
  }

  // POST /sprint-sheet/:taskId/hours {log_date (YYYY-MM-DD), hours}
  // -> 201 creado o 200 actualizado {data: {log_date, hours}}.
  // Ver back/social/internal/handler/http/hours_handler.go:30 LogHours.
  Future<SprintHours> logHours({
    required String taskId,
    required String logDate,
    required double hours,
  }) async {
    final tid = taskId.trim();
    if (tid.isEmpty) {
      throw SocialApiException('taskId vacío.', code: 'invalid_task_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/sprint-sheet/$tid/hours'),
            headers: _headers(json: true),
            body: jsonEncode({'log_date': logDate.trim(), 'hours': hours}),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201 && res.statusCode != 200) {
      throw _toError(res);
    }
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    final data =
        (body['data'] as Map<String, dynamic>?) ?? const <String, dynamic>{};
    return SprintHours.fromJson(data);
  }

  // GET /sprint-sheet/:taskId/hours -> 200 {data: [...], total_hours}.
  Future<SprintHoursList> listHours(String taskId) async {
    final tid = taskId.trim();
    if (tid.isEmpty) {
      throw SocialApiException('taskId vacío.', code: 'invalid_task_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$baseUrl/sprint-sheet/$tid/hours'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return SprintHoursList.fromJson(body);
  }

  // GET /groups/:id/discord-config -> 200 {server_name, invite_url,
  // webhook_url?}. 404 discord_not_configured si nunca se configuró.
  // Solo miembros. Ver discord_handler.go GetConfig.
  Future<DiscordConfig?> getDiscordConfig(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .get(
            Uri.parse('$baseUrl/groups/$id/discord-config'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode == 404) return null;
    if (res.statusCode != 200) throw _toError(res);
    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return DiscordConfig.fromJson(body);
  }

  // PUT /groups/:id/discord-config {server_name, invite_url, webhook_url?}
  // -> 200. Solo admin. No existe lectura: la UI es formulario de alta.
  // Ver back/social/internal/handler/http/discord_handler.go:38 PutConfig.
  Future<void> updateDiscordConfig({
    required String groupId,
    required String serverName,
    required String inviteUrl,
    String? webhookUrl,
  }) async {
    final payload = <String, dynamic>{
      'server_name': serverName.trim(),
      'invite_url': inviteUrl.trim(),
      if (webhookUrl != null && webhookUrl.trim().isNotEmpty)
        'webhook_url': webhookUrl.trim(),
    };
    final res = await AuthedHttp.run(
      () => _client
          .put(
            Uri.parse('$baseUrl/groups/${groupId.trim()}/discord-config'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 200) throw _toError(res);
  }

  // POST /groups/:id/meetings
  // {title required, description?, scheduled_at RFC3339 required, notify_discord?}
  // -> 201 {meeting_id}. No existe GET /groups/:id/meetings en el backend.
  Future<String> createMeeting({
    required String groupId,
    required String title,
    String? description,
    required DateTime scheduledAtUtc,
    List<String> attendees = const [],
  }) async {
    final payload = <String, dynamic>{
      'title': title.trim(),
      if (description != null && description.trim().isNotEmpty)
        'description': description.trim(),
      if (attendees.isNotEmpty)
        'attendees': normalizeMeetingAttendees(attendees),
      'scheduled_at': scheduledAtUtc.toUtc().toIso8601String(),
    };
    debugPrint('[Social] creando reunión en grupo $groupId');
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/${groupId.trim()}/meetings'),
            headers: _headers(json: true),
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10)),
    );
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

  // POST /groups/:id/leave -> 204. Solo miembros.
  // Si eres el único admin con más miembros, el backend responde 400
  // (debes transferir antes). Si eres el último miembro, el grupo se elimina.
  // Ver back/social/internal/handler/http/group_handler.go:277 LeaveGroup.
  Future<void> leaveGroup(String groupId) async {
    final id = groupId.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/$id/leave'),
            headers: _headers(json: true),
            body: jsonEncode(const <String, dynamic>{}),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 204 && res.statusCode != 200) throw _toError(res);
    notifyGroupsChanged();
  }

  // POST /groups/:id/join {invite_token} -> 201. Requiere groupId + token.
  // Sin resolver token->grupo no hay UX "solo código" (se reporta).
  // Ver back/social/internal/handler/http/group_handler.go:151 Join.
  Future<void> joinGroup({
    required String groupId,
    required String inviteToken,
  }) async {
    final id = groupId.trim();
    final token = inviteToken.trim();
    if (id.isEmpty) {
      throw SocialApiException('groupId vacío.', code: 'invalid_group_id');
    }
    if (token.isEmpty) {
      throw SocialApiException('invite_token requerido.', code: 'invalid_body');
    }
    final res = await AuthedHttp.run(
      () => _client
          .post(
            Uri.parse('$baseUrl/groups/$id/join'),
            headers: _headers(json: true),
            body: jsonEncode(<String, dynamic>{'invite_token': token}),
          )
          .timeout(const Duration(seconds: 10)),
    );
    if (res.statusCode != 201 && res.statusCode != 200) throw _toError(res);
    notifyGroupsChanged();
  }

  void dispose() {
    _client.close();
  }
}

/// Mismo formato, normalización y máximo que parseAttendees del servicio Social.
List<String> normalizeMeetingAttendees(Iterable<String> values) {
  final emails = values
      .map((e) => e.trim().toLowerCase())
      .where((e) => e.isNotEmpty)
      .toSet()
      .toList();
  final pattern = RegExp(r'^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$');
  if (emails.any((e) => e.length > 255 || !pattern.hasMatch(e))) {
    throw const FormatException('Ingresa un correo válido.');
  }
  if (emails.length > 50) {
    throw const FormatException('Puedes invitar hasta 50 personas.');
  }
  return emails;
}
