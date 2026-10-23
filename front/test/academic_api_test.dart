import 'support/service_uri.dart';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/features/academic/academic_api.dart';
import 'package:taller_integracion_front/features/academic/academic_models.dart';
import 'package:taller_integracion_front/features/academic/academic_mappers.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

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

  test('listCurricula usa Bearer y parsea items', () async {
    final api = await apiWith(
      MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.servicePath, '/curricula');
        expect(request.headers['Authorization'], 'Bearer jwt-test');
        return http.Response(
          jsonEncode({
            'items': [
              {'id': 'm1', 'user_id': 'u', 'name': 'Malla 2026'},
            ],
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
    );
    final all = await api.listCurricula();
    expect(all, hasLength(1));
    expect(all.single.id, 'm1');
  });

  test('createSubject envía code/credits y devuelve id', () async {
    final api = await apiWith(
      MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.servicePath, '/curricula/m1/subjects');
        final body = jsonDecode(request.body) as Map<String, dynamic>;
        expect(body['name'], 'Álgebra');
        expect(body['code'], 'MAT-101');
        expect(body['credits'], 5);
        expect(body['status'], 'in_progress');
        return http.Response(jsonEncode({'id': 's1'}), 201);
      }),
    );
    final id = await api.createSubject(
      curriculumId: 'm1',
      name: 'Álgebra',
      code: 'MAT-101',
      professor: 'M. Soto',
      room: 'CJP11-204',
      credits: 5,
    );
    expect(id, 's1');
  });

  test('fan-out: bloque de 2 días -> 1 GET + 2 POSTs', () async {
    final posts = <Map<String, dynamic>>[];
    final api = await apiWith(
      MockClient((request) async {
        if (request.method == 'GET') {
          return http.Response(jsonEncode({'items': []}), 200);
        }
        posts.add(jsonDecode(request.body) as Map<String, dynamic>);
        return http.Response(jsonEncode({'id': 'h${posts.length}'}), 201);
      }),
    );
    final drafts = expandScheduleEntry('Lun / Mié · 10:00–11:30');
    final res = await api.createSchedulesFanOut(
      subjectId: 's1',
      days: drafts.map((d) => d.dayOfWeek).toList(),
      startTime: '10:00',
      endTime: '11:30',
    );
    expect(res.hasFailures, isFalse);
    expect(res.createdIds, ['h1', 'h2']);
    expect(posts.map((p) => p['day_of_week']), [1, 3]);
    expect(posts.first['start_time'], '10:00');
  });

  test(
    'fan-out parcial: falla el 2º -> error visible + reintento sin duplicar',
    () async {
      var posts = 0;
      final api = await apiWith(
        MockClient((request) async {
          // GET agregado: día 1 ya creado ("HH:MM:SS" de la API).
          if (request.method == 'GET') {
            return http.Response(
              jsonEncode({
                'items': [
                  {
                    'id': 'h1',
                    'subject_id': 's1',
                    'day_of_week': 1,
                    'start_time': '10:00:00',
                    'end_time': '11:30:00',
                  },
                ],
              }),
              200,
            );
          }
          posts++;
          if (posts == 2) {
            return http.Response(
              jsonEncode({
                'error': {'code': 'internal_error', 'message': 'boom'},
              }),
              500,
            );
          }
          return http.Response(jsonEncode({'id': 'h-ok'}), 201);
        }),
      );
      // Primer intento (sin nada creado): falla el día 3.
      final first = await api.createSchedulesFanOut(
        subjectId: 's1',
        days: [1, 3],
        startTime: '10:00',
        endTime: '11:30',
        skipExisting: false,
      );
      expect(first.createdIds, hasLength(1));
      expect(first.failed.keys, [3]);
      expect(first.failed[3], contains('boom'));

      // Reintento: verifica por GET, omite el día 1, no duplica.
      posts = 0;
      final retry = await api.createSchedulesFanOut(
        subjectId: 's1',
        days: [1, 3],
        startTime: '10:00',
        endTime: '11:30',
      );
      expect(retry.skippedDays, [1]);
      expect(retry.createdIds, ['h-ok']);
      expect(retry.hasFailures, isFalse);
    },
  );

  test('401 mapea unauthorized y sin sesión lanza no_session', () async {
    final api = await apiWith(
      MockClient((request) async {
        return http.Response(
          jsonEncode({
            'error': {'code': 'unauthorized', 'message': 'expirado'},
          }),
          401,
        );
      }),
    );
    try {
      await api.listCurricula();
      fail('debió lanzar');
    } on AcademicApiException catch (e) {
      expect(e.code, 'unauthorized');
      expect(e.statusCode, 401);
    }

    await SessionManager.clear();
    final anon = AcademicApi(
      client: MockClient((_) async => http.Response('{}', 200)),
      baseUrl: 'http://acad',
    );
    addTearDown(anon.dispose);
    try {
      await anon.listCurricula();
      fail('debió lanzar');
    } on AcademicApiException catch (e) {
      expect(e.code, 'no_session');
    }
  });

  test('modelos toleran null y normalizan vacíos', () {
    final s = AcademicSubject.fromJson({
      'id': 's1',
      'curriculum_id': 'm1',
      'name': 'S',
      'code': '  ',
      'status': 'approved',
    });
    expect(s.code, isNull);
    expect(s.professor, isNull);
    expect(s.status, 'approved');
    final slot = AcademicScheduleSlot.fromJson({
      'id': 'h',
      'subject_id': 's1',
      'day_of_week': 3,
      'start_time': '10:00:00',
      'end_time': '11:30:00',
    });
    expect(hhMmToMinutes(slot.startTime), 600);
    expect(slot.modality, isNull);
  });
}
