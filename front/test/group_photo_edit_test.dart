import 'support/service_uri.dart';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/groups/group_detail_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';
const _photo = 'data:image/png;base64,PHVuYS1mb3Rv';
const _thumb = 'data:image/png;base64,dGh1bWI=';

Map<String, dynamic> _groupJson({required String role, String? photo}) {
  final json = <String, dynamic>{
    'id': _gid,
    'name': 'Grupo Real',
    'description': 'Desc',
    'owner_user_id': 'u',
    'notes_restricted_to_staff': false,
    'created_at': '2026-08-11T12:00:00Z',
    'role': role,
    'member_count': 2,
  };
  if (photo != null) {
    json['photo_url'] = photo;
    json['photo_thumb_url'] = _thumb;
  }
  return json;
}

http.Response _json(Object body, [int status = 200]) => http.Response(
  jsonEncode(body),
  status,
  headers: {'content-type': 'application/json'},
);

Future<void> _pump(WidgetTester tester, SocialService service) async {
  tester.view.physicalSize = const Size(1280, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: GroupDetailScreen(
        group: const <String, dynamic>{'id': _gid, 'name': 'Nav', 'members': 9},
        service: service,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

SocialService _detailService({required String role, String? photo}) {
  return SocialService(
    client: MockClient((request) async {
      final path = request.url.servicePath;
      if (request.method == 'GET' && path == '/groups/$_gid') {
        return _json(_groupJson(role: role, photo: photo));
      }
      return _json(<String, dynamic>{'error': 'not_found'}, 404);
    }),
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('updateGroup envía ambas fotos cuando updatePhoto=true', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    late Map<String, dynamic> sent;
    final service = SocialService(
      client: MockClient((request) async {
        expect(request.method, 'PATCH');
        expect(request.url.servicePath, '/groups/$_gid');
        sent = jsonDecode(request.body) as Map<String, dynamic>;
        return _json(_groupJson(role: 'admin', photo: _photo));
      }),
    );
    addTearDown(service.dispose);

    final updated = await service.updateGroup(
      groupId: _gid,
      name: '  Renombrado  ',
      description: '  nueva  ',
      photoUrl: _photo,
      photoThumbUrl: _thumb,
      updatePhoto: true,
    );

    expect(sent['name'], 'Renombrado');
    expect(sent['description'], 'nueva');
    expect(sent['photo_url'], _photo);
    expect(sent['photo_thumb_url'], _thumb);
    expect(updated.photoUrl, _photo);
    expect(updated.photoThumbUrl, _thumb);
  });

  test('updateGroup sin updatePhoto no envía campos de foto', () async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    late Map<String, dynamic> sent;
    final service = SocialService(
      client: MockClient((request) async {
        sent = jsonDecode(request.body) as Map<String, dynamic>;
        return _json(_groupJson(role: 'admin'));
      }),
    );
    addTearDown(service.dispose);

    await service.updateGroup(groupId: _gid, name: 'Solo nombre');

    expect(sent.containsKey('photo_url'), isFalse);
    expect(sent.containsKey('photo_thumb_url'), isFalse);
  });

  testWidgets('solo el admin ve el acceso a editar la foto del grupo', (
    tester,
  ) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = _detailService(role: 'admin', photo: _photo);
    addTearDown(service.dispose);

    await _pump(tester, service);

    expect(find.byTooltip('Editar grupo'), findsOneWidget);
    await tester.tap(find.byTooltip('Editar grupo'));
    await tester.pumpAndSettle();

    expect(find.text('EDITAR GRUPO'), findsOneWidget);
    expect(find.text('FOTO DEL GRUPO'), findsOneWidget);
    expect(find.byKey(const Key('group-photo-change')), findsOneWidget);
    expect(find.byKey(const Key('group-photo-remove')), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('un miembro no ve el acceso a editar el grupo', (tester) async {
    SessionManager.saveSession('jwt-test', {'id': 'u'});
    addTearDown(SessionManager.clear);
    final service = _detailService(role: 'member');
    addTearDown(service.dispose);

    await _pump(tester, service);

    expect(find.byTooltip('Editar grupo'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
