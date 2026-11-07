import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

const _noteId = '11111111-1111-4111-8111-111111111111';
const _cloneId = '22222222-2222-4222-8222-222222222222';

http.Response _json(Object body, int status) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

void main() {
  setUp(() => SharedPreferences.setMockInitialValues({}));

  Future<NotesApi> apiWith(MockClient client) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final api = NotesApi(client: client, baseUrl: 'http://notes');
    addTearDown(api.dispose);
    return api;
  }

  test('copyNote clona con idempotency key y devuelve el nuevo id', () async {
    final api = await apiWith(
      MockClient((req) async {
        expect(req.method, 'POST');
        expect(req.url.path, '/notes/$_noteId/copy');
        expect(req.headers['Authorization'], 'Bearer jwt-test');
        expect(req.headers['X-Idempotency-Key'], 'k1');
        return _json({
          'note_id': _cloneId,
          'forked_from_note_id': _noteId,
          'title': 'Álgebra',
          'version': 1,
        }, 201);
      }),
    );
    final copy = await api.copyNote(_noteId, idempotencyKey: 'k1');
    expect(copy.noteId, _cloneId);
    expect(copy.forkedFromNoteId, _noteId);
    expect(copy.title, 'Álgebra');
  });

  test('copyNote de un apunte borrado por su autor: 410 claro', () async {
    final api = await apiWith(
      MockClient(
        (_) async => _json({
          'error': {
            'code': 'note_deleted',
            'message': 'Apunte no disponible (eliminado por el autor)',
          },
        }, 410),
      ),
    );
    await expectLater(
      api.copyNote(_noteId),
      throwsA(
        isA<NotesApiException>()
            .having((e) => e.statusCode, 'status', 410)
            .having((e) => e.code, 'code', 'note_deleted')
            .having((e) => e.message, 'message', contains('eliminó')),
      ),
    );
  });

  test('copyNote sin Drive conectado: 403 pide conectar Drive', () async {
    final api = await apiWith(
      MockClient(
        (_) async => _json({
          'error': {
            'code': 'forbidden',
            'message': 'Conecte o renueve su Google Drive',
          },
        }, 403),
      ),
    );
    await expectLater(
      api.copyNote(_noteId),
      throwsA(
        isA<NotesApiException>().having(
          (e) => e.message,
          'message',
          contains('Google Drive'),
        ),
      ),
    );
  });
}
