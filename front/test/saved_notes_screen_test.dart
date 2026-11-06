import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';
import 'package:taller_integracion_front/features/notes/saved_notes_screen.dart';

const _alive = '11111111-1111-4111-8111-111111111111';
const _gone = '22222222-2222-4222-8222-222222222222';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Map<String, Object?> _availableItem() => {
  'saved_id': 's-alive',
  'note_id': _alive,
  'title': 'Integrales dobles',
  'is_available': true,
  'user_id': 'aaaaaaaa-0000-4000-8000-000000000001',
  'likes_count': 4,
  'saves_count': 2,
  'is_liked': false,
  'is_saved': true,
  'is_own': false,
  'saved_at': '2026-11-05T12:00:00Z',
};

Map<String, Object?> _deletedItem() => {
  'saved_id': 's-gone',
  'note_id': _gone,
  'title': 'Redes - Modelo OSI',
  'is_available': false,
  'unavailable_reason': 'deleted',
  'user_id': 'bbbbbbbb-0000-4000-8000-000000000002',
  'is_saved': true,
  'deleted_at': '2026-11-05T13:00:00Z',
};

class _Backend {
  final List<String> calls = [];
  List<Map<String, Object?>> items = [_availableItem(), _deletedItem()];

  Future<http.Response> handle(http.Request req) async {
    calls.add('${req.method} ${req.url.path}?${req.url.query}');
    final path = req.url.path;
    if (path == '/notes/saved') {
      final q = req.url.queryParameters;
      var out = items;
      if (q['availability'] == 'unavailable') {
        out = out.where((i) => i['is_available'] == false).toList();
      }
      if (q['scope'] == 'others') {
        out = out.where((i) => i['is_own'] != true).toList();
      }
      return _json({'items': out, 'next_cursor': ''}, 200);
    }
    if (req.method == 'DELETE' && path.endsWith('/save')) {
      final id = path.split('/')[2];
      items = items.where((i) => i['note_id'] != id).toList();
      return http.Response('', 204);
    }
    if (path.endsWith('/stats')) {
      return _json({
        'error': {'code': 'note_deleted', 'message': 'x'},
      }, 410);
    }
    if (req.method == 'POST' && path == '/notes/$_alive/copy') {
      return _json({
        'note_id': '33333333-3333-4333-8333-333333333333',
        'forked_from_note_id': _alive,
        'title': 'Integrales dobles',
      }, 201);
    }
    return _json({}, 404);
  }
}

Future<_Backend> _pump(WidgetTester tester, {VoidCallback? onCopied}) async {
  await SessionManager.saveSession('jwt-test', {'id': 'u'});
  addTearDown(SessionManager.clear);
  final backend = _Backend();
  await tester.pumpWidget(
    MaterialApp(
      home: SavedNotesScreen(
        api: NotesApi(client: MockClient(backend.handle), baseUrl: 'http://n'),
        onNoteCopied: onCopied,
      ),
    ),
  );
  await tester.pumpAndSettle();
  return backend;
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  testWidgets('lista disponibles y no disponibles con su etiqueta', (
    tester,
  ) async {
    await _pump(tester);
    expect(find.text('Integrales dobles'), findsOneWidget);
    expect(find.text('4'), findsOneWidget); // contador de likes
    expect(find.byKey(const Key('copy-$_alive')), findsOneWidget);

    final unavailable = find.byKey(const Key('saved-unavailable-$_gone'));
    expect(unavailable, findsOneWidget);
    expect(
      find.descendant(
        of: unavailable,
        matching: find.text('APUNTE NO DISPONIBLE (ELIMINADO POR EL AUTOR)'),
      ),
      findsOneWidget,
    );
    // La tarjeta no disponible solo permite quitarla.
    expect(
      find.descendant(of: unavailable, matching: find.byType(ElevatedButton)),
      findsNothing,
    );
    expect(find.byKey(const Key('copy-$_gone')), findsNothing);
    expect(find.byKey(const Key('like-$_gone')), findsNothing);
    expect(find.byKey(const Key('unsave-$_gone')), findsOneWidget);
  });

  testWidgets('quitar un guardado no disponible lo saca de la lista', (
    tester,
  ) async {
    final backend = await _pump(tester);
    await tester.ensureVisible(find.byKey(const Key('unsave-$_gone')));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('unsave-$_gone')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('saved-unavailable-$_gone')), findsNothing);
    expect(backend.calls, contains('DELETE /notes/$_gone/save?'));
    expect(find.text('Quitado de guardados.'), findsOneWidget);
  });

  testWidgets('filtro "No disponibles" pide availability=unavailable', (
    tester,
  ) async {
    final backend = await _pump(tester);
    await tester.tap(find.byKey(const Key('availability-No disponibles')));
    await tester.pumpAndSettle();
    expect(backend.calls.last, contains('availability=unavailable'));
    expect(find.text('Integrales dobles'), findsNothing);
    expect(find.byKey(const Key('saved-unavailable-$_gone')), findsOneWidget);
  });

  testWidgets('copiar a mis apuntes avisa y refresca', (tester) async {
    var copied = 0;
    final backend = await _pump(tester, onCopied: () => copied++);
    await tester.tap(find.byKey(const Key('copy-$_alive')));
    await tester.pumpAndSettle();
    expect(backend.calls, contains('POST /notes/$_alive/copy?'));
    expect(copied, 1);
    expect(
      find.text('Copia guardada en tus apuntes y en tu Drive.'),
      findsOneWidget,
    );
  });

  testWidgets('sin desbordes en 360 px con texto al 200 %', (tester) async {
    tester.view.physicalSize = const Size(360, 800);
    tester.view.devicePixelRatio = 1;
    tester.platformDispatcher.textScaleFactorTestValue = 2;
    addTearDown(() {
      tester.view.resetPhysicalSize();
      tester.view.resetDevicePixelRatio();
      tester.platformDispatcher.clearTextScaleFactorTestValue();
    });
    await _pump(tester);
    expect(tester.takeException(), isNull);
  });
}
