import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/groups/group_detail_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';

Future<void> _pumpDetail(
  WidgetTester tester,
  SocialService service,
) async {
  tester.view.physicalSize = const Size(1280, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(
      home: GroupDetailScreen(
          group: const <String, dynamic>{
            'id': _gid,
            'name': 'Nombre Navegación (no actualizado)',
            'subject': 'NAV',
            'members': 99,
            'role': 'member',
          },
          service: service)));
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('cabecera muestra GET /groups/:id real, no el Map',
      (tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        if (request.method == 'GET' && request.url.path == '/groups/$_gid') {
          return http.Response(
            jsonEncode(<String, dynamic>{
              'id': _gid,
              'name': 'Grupo Real Backend',
              'description': 'Desc real',
              'owner_user_id': 'u',
              'notes_restricted_to_staff': false,
              'created_at': '2026-08-11T12:00:00Z',
              'role': 'admin',
            }),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        // Tabs (workspace) no se navegan en este test; respuesta mínima.
        return http.Response(
            jsonEncode(<String, dynamic>{
              'group': {
                'id': _gid,
                'name': 'Grupo Real Backend',
                'role': 'admin'
              },
              'kanban': {'todo': [], 'in_progress': [], 'done': []},
              'sprint_sheet': {'sheets': [], 'tasks': []},
              'meetings': {'upcoming': []},
              'chat': {'provider': 'stream', 'token_endpoint': ''},
            }),
            200,
            headers: {'content-type': 'application/json'});
      }),
    );
    addTearDown(service.dispose);

    await _pumpDetail(tester, service);

    expect(find.text('GRUPO REAL BACKEND'), findsOneWidget);
    expect(find.textContaining('Desc real'), findsOneWidget);
    expect(find.text('Nombre Navegación (no actualizado)'), findsNothing);
    expect(find.textContaining('99 integrantes'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('cabecera con error muestra reintento real', (tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        return http.Response(
            jsonEncode({
              'error': {'code': 'forbidden', 'message': 'denegado'}
            }),
            403,
            headers: {'content-type': 'application/json'});
      }),
    );
    addTearDown(service.dispose);

    await _pumpDetail(tester, service);

    expect(find.text('NO SE PUDO CARGAR EL GRUPO (REAL)'), findsOneWidget);
    expect(find.text('REINTENTAR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
