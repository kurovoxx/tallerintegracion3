import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/note_id_search_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

const _id = '11111111-1111-4111-8111-111111111111';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Map<String, Object> _err(String code, String message) => {
  'error': {'code': code, 'message': message},
};

Future<void> _pump(WidgetTester tester, MockClient client) async {
  await SessionManager.saveSession('jwt-test', {'id': 'u'});
  addTearDown(SessionManager.clear);
  await tester.pumpWidget(
    MaterialApp(
      home: NoteIdSearchScreen(
        api: NotesApi(client: client, baseUrl: 'http://notes'),
      ),
    ),
  );
}

Future<void> _searchFor(WidgetTester tester, String text) async {
  await tester.enterText(find.byKey(const Key('note-id-search-field')), text);
  await tester.tap(find.byKey(const Key('note-id-search-button')));
  await tester.pumpAndSettle();
}

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  group('extractNoteId', () {
    test('acepta el UUID solo, con espacios y en mayúsculas', () {
      expect(extractNoteId('  $_id  '), _id);
      expect(extractNoteId(_id.toUpperCase()), _id);
    });
    test('extrae el UUID de un enlace', () {
      expect(extractNoteId('https://ti3.example/notes/$_id?x=1'), _id);
    });
    test('rechaza texto sin UUID', () {
      expect(extractNoteId('calculo integral'), isNull);
      expect(extractNoteId('1234-5678'), isNull);
    });
  });

  testWidgets('encuentra un apunte público por ID', (tester) async {
    await _pump(
      tester,
      MockClient((req) async {
        expect(req.url.path, '/notes/$_id');
        return _json({
          'id': _id,
          'user_id': 'autor',
          'title': 'Física I',
          'content': '# Cinemática',
          'visibility': 'public',
          'likes_count': 3,
          'saves_count': 1,
          'is_own': false,
        }, 200);
      }),
    );
    await _searchFor(tester, 'https://app/notes/${_id.toUpperCase()}');
    expect(find.byKey(const Key('note-search-result')), findsOneWidget);
    expect(find.text('Física I'), findsOneWidget);
    expect(find.text('3 LIKES'), findsOneWidget);
    expect(find.byKey(const Key('note-search-copy')), findsOneWidget);
  });

  testWidgets('404: no existe o sin permiso', (tester) async {
    await _pump(
      tester,
      MockClient(
        (_) async => _json(_err('not_found', 'nota no encontrada'), 404),
      ),
    );
    await _searchFor(tester, _id);
    expect(find.text('SIN RESULTADOS'), findsOneWidget);
    expect(find.textContaining('no tienes permiso'), findsOneWidget);
  });

  testWidgets('410: apunte eliminado por su autor', (tester) async {
    await _pump(
      tester,
      MockClient(
        (_) async => _json(
          _err('note_deleted', 'Apunte no disponible (eliminado por el autor)'),
          410,
        ),
      ),
    );
    await _searchFor(tester, _id);
    expect(find.text('APUNTE NO DISPONIBLE'), findsOneWidget);
    expect(find.textContaining('su autor lo eliminó'), findsOneWidget);
  });

  testWidgets('texto sin ID no llama al backend', (tester) async {
    var calls = 0;
    await _pump(
      tester,
      MockClient((_) async {
        calls++;
        return _json({}, 200);
      }),
    );
    await _searchFor(tester, 'no es un id');
    expect(calls, 0);
    expect(find.text('ID INVÁLIDO'), findsOneWidget);
  });
}
