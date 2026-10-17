import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/notes_sharing_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/groups/group_notes_tab.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _noteId = '22222222-2222-4222-8222-222222222222';

/// Backend de notas fake con estado: compartir/unshare alternan el listado.
class _FakeNotesApi {
  bool shared;
  bool failList;

  _FakeNotesApi({this.shared = false, this.failList = false});

  http.Response _json(int code, Object body) => http.Response(
    jsonEncode(body),
    code,
    headers: {'content-type': 'application/json'},
  );

  Future<http.Response> handle(http.Request req) async {
    final p = req.url.path;
    if (req.method == 'GET' && p.endsWith('/groups/$_gid/notes')) {
      if (failList) {
        return _json(403, {
          'error': {'code': 'forbidden', 'message': 'denegado'},
        });
      }
      final notes = shared
          ? [
              {
                'id': _noteId,
                'title': 'Nota compartida',
                'visibility': 'private',
                'likes_count': 3,
                'version': 1,
                'user_id': 'u',
              },
            ]
          : [];
      return _json(200, {'notes': notes, 'next_cursor': ''});
    }
    if (req.method == 'GET' && p.endsWith('/notes/me')) {
      return _json(200, {
        'notes': [
          {'id': _noteId, 'title': 'Mi nota', 'visibility': 'private'},
        ],
        'next_cursor': '',
      });
    }
    if (req.method == 'POST' && p.endsWith('/notes/$_noteId/share')) {
      shared = true;
      return _json(201, {'shared_note_id': 's1'});
    }
    if (req.method == 'DELETE' && p.contains('/notes/shared/')) {
      shared = false;
      return http.Response('', 204);
    }
    if (req.method == 'GET' && p.endsWith('/notes/$_noteId')) {
      return _json(200, {
        'id': _noteId,
        'title': 'Nota compartida',
        'content': 'cuerpo markdown',
        'visibility': 'private',
      });
    }
    return http.Response('not found', 404);
  }
}

Future<void> _pumpTab(WidgetTester tester, NotesSharingService service) async {
  await tester.pumpWidget(
    MaterialApp(
      home: Scaffold(body: GroupNotesTab(groupId: _gid, service: service)),
    ),
  );
  await tester.pumpAndSettle();
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    SessionManager.saveSession('jwt-test', {'id': 'u'});
  });
  tearDown(SessionManager.clear);

  testWidgets('lista las notas compartidas del grupo', (tester) async {
    final api = _FakeNotesApi(shared: true);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);

    expect(find.text('NOTAS COMPARTIDAS'), findsOneWidget);
    expect(find.text('Nota compartida'), findsOneWidget);
    expect(find.text('ABRIR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('estado vacío cuando el grupo no tiene notas', (tester) async {
    final api = _FakeNotesApi(shared: false);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);

    expect(
      find.text('Aún no hay notas compartidas con este grupo.'),
      findsOneWidget,
    );
    expect(find.text('COMPARTIR UNA NOTA'), findsOneWidget);
  });

  testWidgets('error de carga muestra REINTENTAR y recupera', (tester) async {
    final api = _FakeNotesApi(shared: true, failList: true);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);
    expect(find.text('REINTENTAR'), findsOneWidget);
    expect(find.text('Nota compartida'), findsNothing);

    api.failList = false;
    await tester.tap(find.text('REINTENTAR'));
    await tester.pumpAndSettle();
    expect(find.text('Nota compartida'), findsOneWidget);
  });

  testWidgets('compartir (link) refresca y permite dejar de compartir', (
    tester,
  ) async {
    final api = _FakeNotesApi(shared: false);
    final service = NotesSharingService(client: MockClient(api.handle));
    addTearDown(service.dispose);

    await _pumpTab(tester, service);
    expect(find.text('COMPARTIR UNA NOTA'), findsOneWidget);

    // Abrir diálogo de compartir (carga mis notas) y confirmar en modo LINK.
    await tester.tap(find.text('COMPARTIR UNA NOTA'));
    await tester.pumpAndSettle();
    expect(find.text('LINK'), findsOneWidget);
    expect(find.text('RESTRICTED'), findsOneWidget);

    await tester.tap(find.text('COMPARTIR'));
    await tester.pumpAndSettle();

    // Refresco: la nota ya aparece como compartida + acción de unshare.
    expect(find.text('Nota compartida'), findsOneWidget);
    expect(find.text('DEJAR DE COMPARTIR'), findsOneWidget);

    // Unshare con confirmación.
    await tester.tap(find.text('DEJAR DE COMPARTIR'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('RETIRAR'));
    await tester.pumpAndSettle();

    expect(find.text('Nota compartida'), findsNothing);
    expect(
      find.text('Aún no hay notas compartidas con este grupo.'),
      findsOneWidget,
    );
  });
}
