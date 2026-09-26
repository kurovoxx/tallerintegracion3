import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/groups/groups_screen.dart';

Map<String, dynamic> _overviewJson(List<Map<String, dynamic>> groups) {
  return <String, dynamic>{
    'user_id': 'u-test',
    'sidebar': {
      'groups': [
        for (final g in groups)
          {
            'group_id': g['group_id'],
            'name': g['name'],
            'role': g['role'],
          },
      ],
    },
    'groups': groups,
    'stats': {
      'groups_count': groups.length,
      'admin_groups_count':
          groups.where((g) => g['role'] == 'admin').length,
    },
  };
}

List<Map<String, dynamic>> _baseGroups() => <Map<String, dynamic>>[
      <String, dynamic>{
        'group_id': '11111111-1111-1111-1111-111111111111',
        'name': 'Cálculo II - Grupo Alpha',
        'description': 'MAT1002',
        'role': 'admin',
        'member_count': 5,
        'joined_at': '2026-08-11T12:00:00Z',
      },
      <String, dynamic>{
        'group_id': '22222222-2222-2222-2222-222222222222',
        'name': 'Bases de Datos - Proyecto',
        'description': 'INF220',
        'role': 'member',
        'member_count': 4,
        'joined_at': '2026-08-12T12:00:00Z',
      },
      <String, dynamic>{
        'group_id': '33333333-3333-3333-3333-333333333333',
        'name': 'Taller Integración III',
        'description': 'INF-360',
        'role': 'member',
        'member_count': 6,
        'joined_at': '2026-08-13T12:00:00Z',
      },
    ];

void main() {
  Future<void> pumpGroups(
    WidgetTester tester,
    Size size, {
    required SocialService service,
  }) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(MaterialApp(home: GroupsScreen(service: service)));
    await tester.pumpAndSettle();
  }

  testWidgets('renderiza grupos reales de GET /me/overview en desktop',
      (tester) async {
    SessionManager.saveSession('jwt-groups-test', {'id': 'u-test'});
    addTearDown(SessionManager.clear);
    final groups = _baseGroups();
    final service = SocialService(
      client: MockClient((request) async {
        if (request.method == 'GET' && request.url.path == '/me/overview') {
          expect(request.headers['Authorization'], 'Bearer jwt-groups-test');
          return http.Response(jsonEncode(_overviewJson(groups)), 200,
              headers: {'content-type': 'application/json'});
        }
        return http.Response('{}', 404);
      }),
    );
    addTearDown(service.dispose);

    await pumpGroups(tester, const Size(1440, 900), service: service);

    expect(find.text('MIS GRUPOS'), findsOneWidget);
    expect(find.text('Cálculo II - Grupo Alpha'), findsOneWidget);
    expect(find.text('MAT1002'), findsOneWidget);
    expect(find.text('INF220'), findsOneWidget);
    expect(find.text('INF-360'), findsOneWidget);
    expect(find.textContaining('5 integrantes'), findsOneWidget);
    expect(find.text('ABRIR'), findsNWidgets(3));
    expect(find.text('INFO'), findsNWidgets(3));
    expect(find.textContaining('3 grupos'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('crea un grupo con POST /groups real en movil', (tester) async {
    SessionManager.saveSession('jwt-groups-test', {'id': 'u-test'});
    addTearDown(SessionManager.clear);
    var groups = _baseGroups();
    final service = SocialService(
      client: MockClient((request) async {
        if (request.method == 'GET' && request.url.path == '/me/overview') {
          return http.Response(jsonEncode(_overviewJson(groups)), 200,
              headers: {'content-type': 'application/json'});
        }
        if (request.method == 'POST' && request.url.path == '/groups') {
          final body = jsonDecode(request.body) as Map<String, dynamic>;
          expect(body['name'], 'Equipo Física');
          groups = [
            ...groups,
            <String, dynamic>{
              'group_id': '44444444-4444-4444-4444-444444444444',
              'name': 'Equipo Física',
              'description': 'General',
              'role': 'admin',
              'member_count': 1,
              'joined_at': '2026-09-25T12:00:00Z',
            },
          ];
          return http.Response(
              jsonEncode(
                  {'group_id': '44444444-4444-4444-4444-444444444444'}),
              201,
              headers: {'content-type': 'application/json'});
        }
        return http.Response('{}', 404);
      }),
    );
    addTearDown(service.dispose);

    await pumpGroups(tester, const Size(390, 844), service: service);

    expect(find.byType(NeobrutalistFab), findsOneWidget);
    await tester.tap(find.byType(NeobrutalistFab));
    await tester.pumpAndSettle();
    expect(find.text('CREAR GRUPO (REAL)'), findsOneWidget);

    await tester.enterText(find.byType(TextField).first, 'Equipo Física');
    await tester.tap(find.text('CREAR GRUPO'));
    await tester.pumpAndSettle();

    expect(find.text('Equipo Física'), findsOneWidget);
    expect(find.textContaining('Grupo creado:'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza sin desbordes', (tester) async {
    SessionManager.saveSession('jwt-groups-test', {'id': 'u-test'});
    addTearDown(SessionManager.clear);
    final service = SocialService(
      client: MockClient((request) async {
        return http.Response(jsonEncode(_overviewJson(_baseGroups())), 200,
            headers: {'content-type': 'application/json'});
      }),
    );
    addTearDown(service.dispose);

    await pumpGroups(tester, const Size(320, 800), service: service);

    expect(find.text('MIS GRUPOS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('sin sesión muestra error real, no grupos demo', (tester) async {
    SessionManager.clear();
    final service = SocialService(
      client: MockClient((request) async {
        return http.Response('{}', 200);
      }),
    );
    addTearDown(service.dispose);

    tester.view.physicalSize = const Size(1440, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(MaterialApp(home: GroupsScreen(service: service)));
    await tester.pumpAndSettle();

    expect(find.text('NO SE PUDO CARGAR TUS GRUPOS'), findsOneWidget);
    expect(find.text('Cálculo II - Grupo Alpha'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
