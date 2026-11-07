import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/notes_sharing_service.dart'
    as sharing;
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

const _note = '11111111-1111-4111-8111-111111111111';
const _group = '22222222-2222-4222-8222-222222222222';

/// Backend que acepta el primer share de (nota, grupo) y responde 409
/// already_shared a los siguientes, como el UNIQUE (note_id, group_id).
MockClient _backend(List<int> statuses) {
  final shared = <String>{};
  return MockClient((req) async {
    final body = jsonDecode(req.body) as Map<String, dynamic>;
    final key = '${req.url.path}|${body['group_id']}';
    if (shared.add(key)) {
      statuses.add(201);
      return http.Response(
        jsonEncode({'shared_note_id': 'sh-1'}),
        201,
        headers: {'content-type': 'application/json'},
      );
    }
    statuses.add(409);
    return http.Response(
      jsonEncode({
        'error': {
          'code': 'already_shared',
          'message': 'Este apunte ya está compartido en este grupo',
        },
      }),
      409,
      headers: {'content-type': 'application/json'},
    );
  });
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
  });
  tearDown(SessionManager.clear);

  test(
    'NotesSharingService: segundo share al mismo grupo -> mensaje claro',
    () async {
      final statuses = <int>[];
      final service = sharing.NotesSharingService(client: _backend(statuses));
      expect(
        await service.shareNote(
          noteId: _note,
          groupId: _group,
          accessMode: 'link',
        ),
        'sh-1',
      );
      await expectLater(
        service.shareNote(
          noteId: _note,
          groupId: _group,
          accessMode: 'restricted',
        ),
        throwsA(
          isA<sharing.NotesApiException>()
              .having((e) => e.statusCode, 'status', 409)
              .having((e) => e.code, 'code', 'already_shared')
              .having(
                (e) => e.message,
                'message',
                'Este apunte ya está compartido en este grupo.',
              ),
        ),
      );
      expect(statuses, [201, 409]);
    },
  );

  test('NotesApi.shareNote: 409 already_shared -> mensaje claro', () async {
    final api = NotesApi(client: _backend(<int>[]), baseUrl: 'http://n');
    await api.shareNote(noteId: _note, groupId: _group, accessMode: 'link');
    await expectLater(
      api.shareNote(noteId: _note, groupId: _group, accessMode: 'link'),
      throwsA(
        isA<NotesApiException>()
            .having((e) => e.code, 'code', 'already_shared')
            .having(
              (e) => e.message,
              'message',
              contains('ya está compartido'),
            ),
      ),
    );
  });

  test('notesFriendlyMessage cubre los códigos de la suite', () {
    expect(notesFriendlyMessage(410, 'note_deleted'), contains('eliminó'));
    expect(notesFriendlyMessage(409, 'already_shared'), contains('compartido'));
    expect(
      notesFriendlyMessage(404, 'not_found'),
      contains('no tienes permiso'),
    );
    expect(notesFriendlyMessage(429, 'rate_limited'), contains('copias'));
    expect(notesFriendlyMessage(-1, null), contains('Sin conexión'));
  });
}
