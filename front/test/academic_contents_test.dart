import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/academic_api.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

// UX-05 (tester): contenidos CRUD + pin/rename por API. Solo tests.
void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  Future<AcademicApi> apiWith(MockClient client) async {
    await SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final api = AcademicApi(client: client, baseUrl: 'http://acad');
    addTearDown(api.dispose);
    return api;
  }

  test('contents: lista ordenada, crea, edita, reordena y elimina', () async {
    final api = await apiWith(
      MockClient((request) async {
        final path = request.url.path;
        if (request.method == 'GET' && path.endsWith('/contents')) {
          return http.Response(
            jsonEncode({
              'items': [
                {
                  'id': 'c1',
                  'subject_id': 's1',
                  'title': 'U1',
                  'body': 'Intro',
                  'position': 1,
                },
                {
                  'id': 'c2',
                  'subject_id': 's1',
                  'title': 'U2',
                  'body': null,
                  'position': 2,
                },
              ],
            }),
            200,
          );
        }
        if (request.method == 'POST' && path.endsWith('/contents')) {
          final body = jsonDecode(request.body) as Map<String, dynamic>;
          expect(body['title'], 'U3');
          expect(body['position'], 3);
          return http.Response(jsonEncode({'id': 'c3'}), 201);
        }
        if (request.method == 'PATCH') {
          return http.Response(jsonEncode({'ok': true}), 200);
        }
        if (request.method == 'DELETE') {
          return http.Response('', 204);
        }
        return http.Response('nope', 404);
      }),
    );
    final items = await api.listContents('s1');
    expect(items.map((c) => c.position).toList(), [1, 2]);
    expect(items.first.body, 'Intro');
    expect(items.last.body, isNull);
    final id = await api.createContent(
      subjectId: 's1',
      title: 'U3',
      position: 3,
    );
    expect(id, 'c3');
    await api.patchContent('c1', {'title': 'U1b'});
    await api.patchContent('c1', {'position': 2});
    await api.deleteContent('c2');
  });

  test('renameGrade envía el nombre (pin eliminado de la UI)', () async {
    final patches = <Map<String, dynamic>>[];
    final api = await apiWith(
      MockClient((request) async {
        expect(request.method, 'PATCH');
        patches.add(jsonDecode(request.body) as Map<String, dynamic>);
        return http.Response(jsonEncode({'ok': true}), 200);
      }),
    );
    await api.renameGrade('g1', 'Certamen 1');
    expect(patches, [
      {'name': 'Certamen 1'},
    ]);
  });
}
