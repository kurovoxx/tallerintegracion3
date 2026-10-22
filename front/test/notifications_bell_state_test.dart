import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/groups/notifications_bell.dart';

// 3_2_18: la campana no duplica popups ni notificaciones y el estado "leída"
// se persiste en el backend (POST /notifications/:id/read) una sola vez.
void main() {
  const n1 = '11111111-1111-4111-8111-111111111111';
  const n2 = '22222222-2222-4222-8222-222222222222';

  late Set<String> readIds;
  late List<String> calls;

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Map<String, dynamic> item(String id, String title) => {
    'id': id,
    'meeting_id': 'm-$id',
    'group_id': 'g-1',
    'title': title,
    'scheduled_at': '2026-10-30T15:00:00Z',
    'read_at': readIds.contains(id) ? '2026-10-22T10:00:00Z' : null,
    'created_at': '2026-10-21T10:00:00Z',
  };

  SocialService buildService({bool duplicate = false}) {
    return SocialService(
      client: MockClient((r) async {
        calls.add('${r.method} ${r.url.path}');
        if (r.method == 'GET' && r.url.path.endsWith('/notifications/me')) {
          return json({
            'data': [
              item(n1, 'Reunión sprint'),
              item(n2, 'Reunión demo'),
              // Respuesta con la misma notificación repetida.
              if (duplicate) item(n1, 'Reunión sprint'),
            ],
          });
        }
        if (r.method == 'POST' && r.url.path.endsWith('/read')) {
          final id = r.url.pathSegments[r.url.pathSegments.length - 2];
          readIds.add(id);
          return json({});
        }
        return json({}, 404);
      }),
    );
  }

  Future<void> pumpBell(WidgetTester t, SocialService service) async {
    t.view.physicalSize = const Size(1200, 900);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(
      MaterialApp(
        home: Scaffold(
          appBar: AppBar(actions: [NotificationsBell(service: service)]),
        ),
      ),
    );
    await t.pumpAndSettle();
  }

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('e30.e30.test', {'id': 'u'});
    readIds = <String>{};
    calls = <String>[];
  });

  tearDown(() async {
    await SessionManager.clear();
  });

  testWidgets(
    'con no leídas la campana abre (el badge no roba el toque) y nunca apila '
    'dos popups',
    (t) async {
      final service = buildService();
      await pumpBell(t, service);
      expect(find.text('2'), findsOneWidget, reason: 'badge con 2 no leídas');

      // Toque en el centro del botón, donde se superpone el badge.
      await t.tap(find.byTooltip('Notificaciones'));
      await t.pumpAndSettle();
      expect(find.text('NOTIFICACIONES'), findsOneWidget);

      // Segundo toque: cae sobre la barrera del popup y lo cierra.
      await t.tap(find.byTooltip('Notificaciones'), warnIfMissed: false);
      await t.pumpAndSettle();
      expect(find.text('NOTIFICACIONES'), findsNothing);

      // Reabrir: exactamente un popup y una fila por notificación.
      await t.tap(find.byTooltip('Notificaciones'));
      await t.pumpAndSettle();
      expect(find.text('NOTIFICACIONES'), findsOneWidget);
      expect(find.text('Reunión sprint'), findsOneWidget);
    },
  );

  testWidgets('notificación repetida en la respuesta se muestra una vez', (
    t,
  ) async {
    final service = buildService(duplicate: true);
    await pumpBell(t, service);
    await t.tap(find.byTooltip('Notificaciones'));
    await t.pumpAndSettle();
    expect(find.text('Reunión sprint'), findsOneWidget);
    expect(find.text('2 NUEVAS'), findsOneWidget);
  });

  testWidgets(
    'marcar como leída persiste UNA vez y actualiza ambos contadores',
    (t) async {
      final service = buildService();
      await pumpBell(t, service);
      await t.tap(find.byTooltip('Notificaciones'));
      await t.pumpAndSettle();
      expect(find.text('2 NUEVAS'), findsOneWidget);

      // Doble toque rápido sobre la misma notificación.
      await t.tap(find.text('Reunión sprint'));
      await t.tap(find.text('Reunión sprint'), warnIfMissed: false);
      await t.pumpAndSettle();

      expect(
        calls.where((c) => c == 'POST /notifications/$n1/read').length,
        1,
        reason: 'un solo POST de leída',
      );
      expect(readIds, {n1});
      expect(find.text('1 NUEVA'), findsOneWidget);

      // Cerrar: la campana recarga desde el backend y conserva el estado.
      await t.tap(find.byTooltip('Cerrar'));
      await t.pumpAndSettle();
      expect(find.text('NOTIFICACIONES'), findsNothing);
      expect(find.text('1'), findsOneWidget);
    },
  );

  testWidgets('desmontar la campana con el popup abierto lo retira', (t) async {
    final service = buildService();
    await pumpBell(t, service);
    await t.tap(find.byTooltip('Notificaciones'));
    await t.pumpAndSettle();
    expect(find.text('NOTIFICACIONES'), findsOneWidget);

    // Navegar a otra vista que ya no contiene la campana.
    await t.pumpWidget(const MaterialApp(home: Scaffold(body: Text('otra'))));
    await t.pumpAndSettle();
    expect(find.text('NOTIFICACIONES'), findsNothing);
    expect(t.takeException(), isNull);
  });
}
