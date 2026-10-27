import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/academic_api.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
    SessionManager.resetInMemory();
  });

  group('E2E cruzado (mock de 4 servicios)', () {
    testWidgets(
      'auth -> social -> academic -> notes flujo completo',
      (tester) async {
        // ------------------------------------------------------------------
        // 1) Auth: register + login → JWT
        //    baseUrl por defecto: https://ti3-brojas.dev.censei.cl/api-auth
        // ------------------------------------------------------------------
        String? accessToken;
        final auth = AuthService(
          client: MockClient((request) async {
            if (request.method == 'POST' &&
                request.url.path == '/api-auth/auth/register') {
              return http.Response(
                jsonEncode({'id': 'u-123'}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            if (request.method == 'POST' &&
                request.url.path == '/api-auth/auth/login') {
              accessToken = 'access-token-e2e';
              return http.Response(
                jsonEncode({
                  'access_token': accessToken,
                  'refresh_token': 'refresh-token-e2e',
                }),
                200,
                headers: {'content-type': 'application/json'},
              );
            }
            return http.Response('{}', 404);
          }),
        );
        addTearDown(auth.dispose);

        await auth.register(
          email: 'e2e@test.invalid',
          password: 'CrossService123!',
          displayName: 'E2E Usuario',
        );
        final login = await auth.login(
          email: 'e2e@test.invalid',
          password: 'CrossService123!',
        );
        expect(login.success, isTrue);
        accessToken = login.data?['access_token'] as String?;
        expect(accessToken, isNotEmpty);

        await SessionManager.saveSession(accessToken!, {'id': 'u-123'});

        // ------------------------------------------------------------------
        // 2) Social: crear grupo + unirse
        //    baseUrl por defecto: https://ti3-brojas.dev.censei.cl/api-social
        // ------------------------------------------------------------------
        String? groupId;
        final social = SocialService(
          client: MockClient((request) async {
            if (request.method == 'POST' &&
                request.url.path == '/api-social/groups') {
              groupId = 'g-456';
              return http.Response(
                jsonEncode({'group_id': groupId}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            if (request.method == 'POST' &&
                request.url.path.contains('/groups/') &&
                request.url.path.endsWith('/join')) {
              return http.Response('', 201);
            }
            return http.Response('{}', 404);
          }),
        );
        addTearDown(social.dispose);

        final createdGroupId = await social.createGroup(name: 'Grupo E2E');
        expect(createdGroupId, groupId);

        await social.joinGroup(groupId: createdGroupId, inviteToken: 'tok-1');

        // ------------------------------------------------------------------
        // 3) Academic (infacadem): crear currículo + materia
        //    baseUrl personalizada: http://academic
        // ------------------------------------------------------------------
        String? curriculumId;
        String? subjectId;
        final academic = AcademicApi(
          client: MockClient((request) async {
            if (request.method == 'POST' &&
                request.url.path == '/curricula') {
              curriculumId = 'c-789';
              return http.Response(
                jsonEncode({'id': curriculumId}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            if (request.method == 'POST' &&
                request.url.path == '/curricula/$curriculumId/subjects') {
              subjectId = 's-101';
              return http.Response(
                jsonEncode({'id': subjectId}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            return http.Response('{}', 404);
          }),
          baseUrl: 'http://academic',
        );
        addTearDown(academic.dispose);

        final createdCurriculumId = await academic.createCurriculum('Malla E2E');
        expect(createdCurriculumId, curriculumId);

        final createdSubjectId = await academic.createSubject(
          curriculumId: createdCurriculumId,
          name: 'Matemática E2E',
          code: 'MAT101',
        );
        expect(createdSubjectId, subjectId);

        // ------------------------------------------------------------------
        // 4) Notes: crear nota + compartir al grupo
        //    baseUrl personalizada: http://notes
        // ------------------------------------------------------------------
        String? noteId;
        final notes = NotesApi(
          client: MockClient((request) async {
            if (request.method == 'POST' && request.url.path == '/notes') {
              noteId = 'n-202';
              return http.Response(
                jsonEncode({'note_id': noteId}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            if (request.method == 'POST' &&
                request.url.path.contains('/notes/') &&
                request.url.path.endsWith('/share')) {
              return http.Response(
                jsonEncode({'shared_note_id': 'sn-303'}),
                201,
                headers: {'content-type': 'application/json'},
              );
            }
            return http.Response('{}', 404);
          }),
          baseUrl: 'http://notes',
        );
        addTearDown(notes.dispose);

        final createdNoteId = await notes.createNote(
          title: 'Apunte E2E',
          content: '# Prueba',
          visibility: 'private',
        );
        expect(createdNoteId, noteId);

        final sharedNoteId = await notes.shareNote(
          noteId: createdNoteId,
          groupId: createdGroupId,
          accessMode: 'restricted',
        );
        expect(sharedNoteId, isNotEmpty);

        // ------------------------------------------------------------------
        // 5) Flujo completado: los 4 servicios respondieron 2xx
        // ------------------------------------------------------------------
        expect(accessToken, isNotEmpty);
        expect(groupId, isNotEmpty);
        expect(curriculumId, isNotEmpty);
        expect(subjectId, isNotEmpty);
        expect(noteId, isNotEmpty);
        expect(sharedNoteId, isNotEmpty);
      },
    );
  });
}
