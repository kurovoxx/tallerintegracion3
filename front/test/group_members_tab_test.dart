import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/groups/group_members_tab.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _adminId = 'aaaaaaaa-0000-4000-8000-000000000001';
const _memberId = 'bbbbbbbb-0000-4000-8000-000000000002';

String _jwtFor(String userId) {
  String enc(Map<String, dynamic> m) =>
      base64Url.encode(utf8.encode(jsonEncode(m))).replaceAll('=', '');
  final exp =
      DateTime.now().add(const Duration(hours: 1)).millisecondsSinceEpoch ~/
      1000;
  return '${enc({'alg': 'HS256'})}.${enc({'user_id': userId, 'exp': exp})}.firma';
}

class _FakeMembersApi {
  _FakeMembersApi(this.members);

  final List<Map<String, dynamic>> members;
  final List<String> actions = <String>[];

  http.Response _json(int code, Object body) => http.Response(
    jsonEncode(body),
    code,
    headers: {'content-type': 'application/json'},
  );

  Future<http.Response> handle(http.Request req) async {
    final p = req.url.path;
    if (req.method == 'GET' && p.endsWith('/groups/$_gid/members')) {
      return _json(200, members);
    }
    if (req.method == 'POST' && p.endsWith('/kick')) {
      actions.add('kick');
      members.removeWhere((m) => p.contains(m['user_id'].toString()));
      return http.Response('', 204);
    }
    if (req.method == 'POST' && p.endsWith('/ban')) {
      actions.add('ban');
      members.removeWhere((m) => p.contains(m['user_id'].toString()));
      return http.Response('', 204);
    }
    return http.Response('not found', 404);
  }
}

_FakeMembersApi _seed() => _FakeMembersApi(<Map<String, dynamic>>[
  {
    'id': 'm1',
    'group_id': _gid,
    'user_id': _adminId,
    'role': 'admin',
    'display_name': 'Ana Admin',
    'email': 'ana@example.com',
    'joined_at': '2026-01-01T00:00:00Z',
  },
  {
    'id': 'm2',
    'group_id': _gid,
    'user_id': _memberId,
    'role': 'member',
    'display_name': 'Beto Miembro',
    'email': 'beto@example.com',
  },
]);

Future<void> _pump(WidgetTester tester, SocialService service) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: GroupMembersTab(groupId: _gid, service: service)),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));
  tearDown(SessionManager.clear);

  testWidgets('admin: lista miembros con rol y acciones de gestión', (
    tester,
  ) async {
    SessionManager.saveSession(_jwtFor(_adminId), {'id': 'x'});
    final api = _seed();
    final service = SocialService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pump(tester, service);

    expect(find.text('Ana Admin'), findsOneWidget);
    expect(find.text('Beto Miembro'), findsOneWidget);
    expect(find.text('ADMIN'), findsOneWidget);
    expect(find.text('MIEMBRO'), findsOneWidget);
    // Solo el usuario actual lleva la etiqueta TÚ.
    expect(find.text('TÚ'), findsOneWidget);
    // Dos fichas -> dos botones VER; gestión solo sobre el otro miembro.
    expect(find.text('VER'), findsNWidgets(2));
    expect(find.text('EXPULSAR'), findsOneWidget);
    expect(find.text('BANEAR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('miembro no admin: sin acciones de gestión', (tester) async {
    SessionManager.saveSession(_jwtFor(_memberId), {'id': 'x'});
    final api = _seed();
    final service = SocialService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pump(tester, service);

    expect(find.text('Beto Miembro'), findsOneWidget);
    expect(find.text('EXPULSAR'), findsNothing);
    expect(find.text('BANEAR'), findsNothing);
  });

  testWidgets('ver abre la ficha del integrante', (tester) async {
    SessionManager.saveSession(_jwtFor(_adminId), {'id': 'x'});
    final api = _seed();
    final service = SocialService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pump(tester, service);
    await tester.tap(find.text('VER').first);
    await tester.pumpAndSettle();

    expect(find.text('FICHA DE INTEGRANTE'), findsOneWidget);
    expect(find.text('CORREO'), findsOneWidget);
    expect(find.text('ROL'), findsOneWidget);
  });

  testWidgets('expulsar llama al endpoint y refresca la lista', (tester) async {
    SessionManager.saveSession(_jwtFor(_adminId), {'id': 'x'});
    final api = _seed();
    final service = SocialService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pump(tester, service);
    await tester.tap(find.text('EXPULSAR'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('CONFIRMAR'));
    await tester.pumpAndSettle();

    expect(api.actions, contains('kick'));
    expect(find.text('Beto Miembro'), findsNothing);
    expect(find.text('Ana Admin'), findsOneWidget);
  });

  testWidgets('banear llama al endpoint y refresca la lista', (tester) async {
    SessionManager.saveSession(_jwtFor(_adminId), {'id': 'x'});
    final api = _seed();
    final service = SocialService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pump(tester, service);
    await tester.tap(find.text('BANEAR'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('CONFIRMAR'));
    await tester.pumpAndSettle();

    expect(api.actions, contains('ban'));
    expect(find.text('Beto Miembro'), findsNothing);
  });
}
