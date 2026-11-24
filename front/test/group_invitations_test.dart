import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/core/widgets/group_invitation_notifications.dart';
import 'package:taller_integracion_front/features/academic/followed_screen.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  tearDown(() async {
    ProfileService.current.value = null;
    await SessionManager.clear();
  });

  testWidgets('invita a un seguido al grupo seleccionado', (tester) async {
    await SessionManager.saveSession('jwt-test', {'id': 'inviter-id'});
    ProfileService.current.value = UserProfile(
      displayName: 'Invitador',
      publicId: 'invitercode1',
      visibility: 'public',
    );
    final profiles = ProfileService(
      client: MockClient((request) async {
        expect(request.url.path, endsWith('/profile/following/invitercode1'));
        return http.Response(
          jsonEncode({
            'data': [
              {
                'user_id': 'followed-user-id',
                'public_id': 'followedcode1',
                'display_name': 'Juanito',
                'followed_at': '2026-11-24T12:00:00Z',
              },
            ],
          }),
          200,
        );
      }),
    );
    final groups = SocialService(
      client: MockClient((request) async {
        if (request.method == 'GET' &&
            request.url.path.endsWith('/groups/me')) {
          return http.Response(
            jsonEncode([
              {'group_id': 'group-id', 'name': 'Grupo Uno', 'role': 'admin'},
            ]),
            200,
          );
        }
        expect(request.method, 'POST');
        expect(request.url.path, endsWith('/groups/group-id/invite'));
        expect(jsonDecode(request.body), {
          'invited_user_id': 'followed-user-id',
        });
        return http.Response('{"invitation_id":"invitation-id"}', 201);
      }),
    );
    addTearDown(profiles.dispose);
    addTearDown(groups.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: FollowedScreen(service: profiles, groupsService: groups),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Juanito'), findsOneWidget);

    await tester.tap(find.text('INVITAR'));
    await tester.pumpAndSettle();
    expect(find.text('GRUPO UNO'), findsOneWidget);

    await tester.tap(find.text('GRUPO UNO'));
    await tester.pumpAndSettle();

    expect(
      find.text('Invitación enviada a Juanito para Grupo Uno.'),
      findsOneWidget,
    );
  });

  test('lista los grupos del usuario con su rol', () async {
    await SessionManager.saveSession('jwt-test', {'id': 'user-id'});
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'GET');
        expect(request.url.path, endsWith('/groups/me'));
        return http.Response(
          jsonEncode([
            {'group_id': 'group-admin', 'name': 'Grupo admin', 'role': 'admin'},
            {
              'group_id': 'group-member',
              'name': 'Grupo miembro',
              'role': 'member',
            },
          ]),
          200,
        );
      }),
    );
    addTearDown(service.dispose);

    final groups = await service.listMyGroups();

    expect(groups.map((group) => group.isAdmin).toList(), [true, false]);
    expect(groups.map((group) => group.name).toList(), [
      'Grupo admin',
      'Grupo miembro',
    ]);
  });

  testWidgets('la invitación muestra acciones y aceptar actualiza la lista', (
    tester,
  ) async {
    await SessionManager.saveSession('jwt-test', {'id': 'invitee-id'});
    var accepted = false;
    final service = SocialService(
      client: MockClient((request) async {
        if (request.method == 'GET' &&
            request.url.path.endsWith('/group-invitations/pending')) {
          return http.Response(
            jsonEncode({
              'data': [
                {
                  'id': 'invitation-id',
                  'group_id': 'group-id',
                  'group_name': 'Grupo de prueba',
                  'invited_by_user_id': 'inviter-id',
                  'invited_by_name': 'Juan',
                  'status': 'pending',
                  'created_at': '2026-11-24T12:00:00Z',
                },
              ],
            }),
            200,
          );
        }
        if (request.method == 'POST' &&
            request.url.path.endsWith(
              '/group-invitations/invitation-id/accept',
            )) {
          accepted = true;
          return http.Response('{"status":"accepted"}', 200);
        }
        throw StateError(
          'Solicitud inesperada: ${request.method} ${request.url}',
        );
      }),
    );
    addTearDown(service.dispose);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(body: GroupInvitationNotifications(service: service)),
      ),
    );
    await tester.pumpAndSettle();

    expect(
      find.textContaining('Juan te invita a Grupo de prueba'),
      findsOneWidget,
    );
    expect(find.text('ACEPTAR'), findsOneWidget);
    expect(find.text('RECHAZAR'), findsOneWidget);

    await tester.tap(find.text('ACEPTAR'));
    await tester.pumpAndSettle();

    expect(accepted, isTrue);
    expect(
      find.textContaining('Juan te invita a Grupo de prueba'),
      findsNothing,
    );
  });
}
