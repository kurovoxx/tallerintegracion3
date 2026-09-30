import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/social_models.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _tid = '22222222-2222-2222-2222-222222222222';

void main() {
  // Mock de prefs (mismo patrón que session_manager_test.dart) para que
  // saveSession/clear funcionen en tests unitarios sin widgets.
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
  });
  tearDown(() async {
    await SessionManager.clear();
  });

  test('getStreamToken parsea token y channel_id (200)', () async {
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, '/groups/$_gid/stream-token');
        expect(request.headers['Authorization'], 'Bearer jwt-test');
        return http.Response(
          jsonEncode({'token': 'tok-abc', 'channel_id': 'group-canal-1'}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final token = await service.getStreamToken(_gid);
    expect(token.token, 'tok-abc');
    expect(token.channelId, 'group-canal-1');
  });

  test('getStreamToken mapea 403 y 503 a errores controlados', () async {
    SocialService serviceFor(int status) => SocialService(
      client: MockClient(
        (request) async => http.Response(
          jsonEncode({'error': 'x', 'code': 'y'}),
          status,
          headers: {'content-type': 'application/json'},
        ),
      ),
    );

    final s403 = serviceFor(403);
    addTearDown(s403.dispose);
    try {
      await s403.getStreamToken(_gid);
      fail('debió lanzar');
    } on SocialApiException catch (e) {
      expect(e.statusCode, 403);
    }

    final s503 = serviceFor(503);
    addTearDown(s503.dispose);
    try {
      await s503.getStreamToken(_gid);
      fail('debió lanzar');
    } on SocialApiException catch (e) {
      expect(e.statusCode, 503);
    }
  });

  test('updateTodo usa PATCH /groups/{id}/todo/{taskId}', () async {
    String? path;
    String? method;
    final service = SocialService(
      client: MockClient((request) async {
        method = request.method;
        path = request.url.path;
        return http.Response(
          jsonEncode({
            'message': 'todo updated successfully',
            'data': {
              'id': _tid,
              'group_id': _gid,
              'board_id': 'b',
              'board_name': 'General',
              'title': 'Nuevo título',
              'status': 'done',
            },
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final updated = await service.updateTodo(
      groupId: _gid,
      taskId: _tid,
      title: 'Nuevo título',
      status: 'done',
    );
    expect(method, 'PATCH');
    expect(path, '/groups/$_gid/todo/$_tid');
    expect(updated.title, 'Nuevo título');
    expect(updated.status, 'done');
  });

  test('deleteTodo usa DELETE /groups/{id}/todo/{taskId}', () async {
    String? path;
    String? method;
    final service = SocialService(
      client: MockClient((request) async {
        method = request.method;
        path = request.url.path;
        return http.Response(
          jsonEncode({'message': 'ok', 'data': {}}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    await service.deleteTodo(groupId: _gid, taskId: _tid);
    expect(method, 'DELETE');
    expect(path, '/groups/$_gid/todo/$_tid');
  });

  test('updateSprintTask usa PATCH con :taskId y delete con :taskId', () async {
    final paths = <String>[];
    final methods = <String>[];
    final service = SocialService(
      client: MockClient((request) async {
        methods.add(request.method);
        paths.add('${request.method} ${request.url.path}');
        return http.Response(
          jsonEncode({
            'message': 'ok',
            'data': {
              'id': _tid,
              'group_id': _gid,
              'sheet_id': 's',
              'sheet_name': 'Sprint 1',
              'title': 'T',
              'assigned_to': 'u',
              'priority': 'alta',
              'status': 'en_proceso',
              'estimated_hours': 3.0,
            },
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    await service.updateSprintTask(
      groupId: _gid,
      taskId: _tid,
      status: 'en_proceso',
    );
    await service.deleteSprintTask(groupId: _gid, taskId: _tid);
    expect(paths, [
      'PATCH /groups/$_gid/sprint-sheet/$_tid',
      'DELETE /groups/$_gid/sprint-sheet/$_tid',
    ]);
  });

  test(
    'listMembers parsea integrantes reales sin nombres inventados',
    () async {
      final service = SocialService(
        client: MockClient((request) async {
          expect(request.url.path, '/groups/$_gid/members');
          return http.Response(
            jsonEncode([
              {
                'id': 'm1',
                'group_id': _gid,
                'user_id': 'aaaaaaaa-0000-4000-8000-000000000001',
                'role': 'admin',
                'joined_at': '2026-08-11T12:00:00Z',
              },
              {
                'id': 'm2',
                'group_id': _gid,
                'user_id': 'bbbbbbbb-0000-4000-8000-000000000002',
                'role': 'member',
              },
            ]),
            200,
            headers: {'content-type': 'application/json'},
          );
        }),
      );
      addTearDown(service.dispose);

      final members = await service.listMembers(_gid);
      expect(members.length, 2);
      expect(members.first.userId, 'aaaaaaaa-0000-4000-8000-000000000001');
      expect(members.first.role, 'admin');
      // La etiqueta no inventa nombres: solo id corto + rol.
      expect(members.first.shortLabel.contains('admin'), isTrue);
      expect(members.first.shortLabel.contains('Sofía'), isFalse);
    },
  );

  test('GroupMember/StreamToken toleran payload parcial', () {
    final m = GroupMember.fromJson({'user_id': 'u1'});
    expect(m.role, 'member');
    expect(m.shortLabel.contains('u1'), isTrue);
    final named = GroupMember.fromJson({
      'user_id': 'u2',
      'role': 'admin',
      'display_name': 'Agustín Vega',
      'email': 'agustin@example.com',
    });
    expect(named.displayLabel, 'Agustín Vega');
    final t = StreamToken.fromJson({
      'token': 't',
      'channel_id': 'c',
      'api_key': 'k',
    });
    expect(t.token, 't');
    expect(t.channelId, 'c');
    expect(t.apiKey, 'k');
  });

  test('regenerateInvite usa POST invite/regenerate', () async {
    String? path;
    String? method;
    final service = SocialService(
      client: MockClient((request) async {
        method = request.method;
        path = request.url.path;
        return http.Response(
          jsonEncode({'new_invite_token': 'tok-nuevo'}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final token = await service.regenerateInvite(_gid);
    expect(method, 'POST');
    expect(path, '/groups/$_gid/invite/regenerate');
    expect(token, 'tok-nuevo');
  });

  test('logHours registra y listHours parsea total', () async {
    final calls = <String>[];
    final service = SocialService(
      client: MockClient((request) async {
        calls.add('${request.method} ${request.url.path}');
        if (request.method == 'POST') {
          return http.Response(
            jsonEncode({
              'message': 'ok',
              'data': {'task_id': _tid, 'log_date': '2026-09-25', 'hours': 2.5},
            }),
            201,
            headers: {'content-type': 'application/json'},
          );
        }
        return http.Response(
          jsonEncode({
            'data': [
              {'task_id': _tid, 'log_date': '2026-09-25', 'hours': 2.5},
            ],
            'total_hours': 2.5,
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    final logged = await service.logHours(
      taskId: _tid,
      logDate: '2026-09-25',
      hours: 2.5,
    );
    expect(logged.hours, 2.5);
    expect(logged.logDate, '2026-09-25');
    final listed = await service.listHours(_tid);
    expect(listed.totalHours, 2.5);
    expect(listed.entries.single.hours, 2.5);
    expect(calls, [
      'POST /sprint-sheet/$_tid/hours',
      'GET /sprint-sheet/$_tid/hours',
    ]);
  });

  test('updateDiscordConfig usa PUT discord-config', () async {
    String? method;
    String? path;
    Map<String, dynamic>? sent;
    final service = SocialService(
      client: MockClient((request) async {
        method = request.method;
        path = request.url.path;
        sent = jsonDecode(request.body) as Map<String, dynamic>;
        return http.Response(
          jsonEncode({'message': 'ok'}),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    addTearDown(service.dispose);

    await service.updateDiscordConfig(
      groupId: _gid,
      serverName: 'Servidor',
      inviteUrl: 'https://discord.gg/x',
    );
    expect(method, 'PUT');
    expect(path, '/groups/$_gid/discord-config');
    expect(sent?['server_name'], 'Servidor');
    expect(sent?.containsKey('webhook_url'), isFalse);
  });
}
