import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';
import 'package:taller_integracion_front/features/academic/public_profile_screen.dart';
import 'package:taller_integracion_front/features/notes/notes_api.dart';

/// Construye un JWT mínimo con el claim `user_id` que lee SessionManager.
/// La firma no se valida en el cliente: solo se decodifica el payload.
String _tokenWithUserId(String? userId) {
  final payload = userId == null
      ? <String, dynamic>{}
      : <String, dynamic>{'user_id': userId};
  final encoded = base64Url.encode(utf8.encode(jsonEncode(payload)));
  return 'eyJhbGciOiJub25lIn0.$encoded.signature';
}

ProfileService _profileService({
  String? publicId = 'a8kp92mqwx7r',
  int publicProfileStatus = 200,
}) {
  return ProfileService(
    client: MockClient((request) async {
      if (request.url.path.endsWith('/google-drive/status')) {
        return http.Response(
          '{"connected":false}',
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      if (request.url.path.contains('/profile/public/')) {
        return http.Response(
          jsonEncode(
            publicProfileStatus == 200
                ? {
                    'public_id': 'b8kp92mqwx7r',
                    'display_name': 'Bea',
                    'visibility': 'public',
                  }
                : {
                    'error': {'code': 'profile_not_found', 'message': 'x'},
                  },
          ),
          publicProfileStatus,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response(
        jsonEncode({
          'display_name': 'Agustín Vega',
          'visibility': 'public',
          'public_id': ?publicId,
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
}

/// Notes solo responde al listado público usado para distinguir un perfil
/// privado (200) de uno inexistente (404).
NotesApi _notesApi({int status = 200, List<String>? requests}) {
  return NotesApi(
    baseUrl: 'http://notes.test',
    client: MockClient((request) async {
      requests?.add(request.url.path);
      return http.Response(
        jsonEncode(
          status == 200
              ? {'notes': [], 'next_cursor': ''}
              : {
                  'error': {'code': 'not_found', 'message': 'x'},
                },
        ),
        status,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
}

Future<void> _pumpProfile(
  WidgetTester tester,
  ProfileService service, {
  NotesApi? notesApi,
}) async {
  tester.view.physicalSize = const Size(1280, 1600);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: ProfileScreen(service: service, notesApi: notesApi),
    ),
  );
  await tester.pumpAndSettle();
}

Future<void> _search(WidgetTester tester, String text) async {
  await tester.enterText(
    find.byKey(const ValueKey('public-profile-search')),
    text,
  );
  await tester.tap(find.byKey(const ValueKey('public-profile-open')));
  await tester.pumpAndSettle();
}

void main() {
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.clear();
    ProfileService.current.value = null;
  });

  tearDown(() async {
    ProfileService.current.value = null;
    await SessionManager.clear();
  });

  const sessionUuid = '550e8400-e29b-41d4-a716-446655440000';

  List<String> captureClipboard(WidgetTester tester) {
    final writes = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>;
          writes.add(args['text'] as String);
        }
        if (call.method == 'Clipboard.getData') {
          return <String, dynamic>{'text': writes.isEmpty ? null : writes.last};
        }
        return null;
      },
    );
    addTearDown(() {
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      );
    });
    return writes;
  }

  testWidgets(
    '«Compartir perfil» copia solo el código público, sin enlace ni UUID',
    (tester) async {
      await SessionManager.saveSession(_tokenWithUserId(sessionUuid), {
        'id': sessionUuid,
      });
      final clipboardWrites = captureClipboard(tester);
      final service = _profileService();
      addTearDown(service.dispose);

      await _pumpProfile(tester, service);
      // La tarjeta «Mi ID de perfil» ya no existe: se comparte con el botón.
      expect(find.text('MI ID DE PERFIL'), findsNothing);
      expect(
        find.byKey(const ValueKey('profile-copy-public-id')),
        findsNothing,
      );
      expect(find.byTooltip('Compartir perfil'), findsOneWidget);

      await tester.tap(find.byKey(const ValueKey('profile-share-link')));
      await tester.pumpAndSettle();

      expect(clipboardWrites, ['a8kp92mqwx7r']);
      expect(clipboardWrites.single, isNot(contains(sessionUuid)));
      expect(clipboardWrites.single, isNot(contains('://')));
      expect(clipboardWrites.single, isNot(contains('sigma-academy')));
      expect(
        find.text('Código del perfil copiado: a8kp92mqwx7r'),
        findsOneWidget,
      );
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('sin public_id no se copia nada y se informa el error', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId(sessionUuid), {
      'id': sessionUuid,
    });
    final clipboardWrites = captureClipboard(tester);
    final service = _profileService(publicId: null);
    addTearDown(service.dispose);

    await _pumpProfile(tester, service);
    await tester.tap(find.byKey(const ValueKey('profile-share-link')));
    await tester.pumpAndSettle();

    expect(clipboardWrites, isEmpty);
    expect(
      find.text(
        'No se pudo obtener el código de tu perfil. Vuelve a iniciar sesión.',
      ),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets('el código copiado se pega en «Buscar perfil» y abre el perfil', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId(sessionUuid), {
      'id': sessionUuid,
    });
    final clipboardWrites = captureClipboard(tester);
    final service = _profileService();
    addTearDown(service.dispose);

    await _pumpProfile(tester, service);
    await tester.tap(find.byKey(const ValueKey('profile-share-link')));
    await tester.pumpAndSettle();
    expect(
      find.text('Ingresa el código del perfil que deseas buscar'),
      findsOneWidget,
    );
    expect(find.text('Pega aquí el enlace del perfil'), findsNothing);
    expect(
      find.text(
        'Encuentra el perfil de otra persona con el código que te compartió.',
      ),
      findsOneWidget,
    );

    await _search(tester, clipboardWrites.single);

    expect(find.byType(PublicProfileScreen), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('buscar con el código (mayúsculas y espacios) abre el perfil', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService();
    addTearDown(service.dispose);

    await _pumpProfile(tester, service);
    expect(find.text('BUSCAR PERFIL'), findsOneWidget);
    await _search(tester, '  B8KP92MQWX7R ');

    expect(find.byType(PublicProfileScreen), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('el antiguo enlace sigma-academy://perfil/ ya no se acepta', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService();
    final requests = <String>[];
    final notes = _notesApi(requests: requests);
    addTearDown(service.dispose);
    addTearDown(notes.dispose);

    await _pumpProfile(tester, service, notesApi: notes);
    await _search(tester, 'sigma-academy://perfil/b8kp92mqwx7r');

    expect(
      find.text(publicIdInputMessage(PublicIdInputError.link)),
      findsOneWidget,
    );
    expect(find.byType(PublicProfileScreen), findsNothing);
    expect(requests, isEmpty);
    expect(tester.takeException(), isNull);
  });

  testWidgets('un perfil privado (Auth 404) con apuntes en Notes se abre', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService(publicProfileStatus: 404);
    final requests = <String>[];
    final notes = _notesApi(requests: requests);
    addTearDown(service.dispose);
    addTearDown(notes.dispose);

    await _pumpProfile(tester, service, notesApi: notes);
    await _search(tester, 'b8kp92mqwx7r');

    expect(requests, ['/public-users/b8kp92mqwx7r/notes']);
    expect(find.byType(PublicProfileScreen), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('un ID inexistente muestra el error y no navega', (tester) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService(publicProfileStatus: 404);
    final notes = _notesApi(status: 404);
    addTearDown(service.dispose);
    addTearDown(notes.dispose);

    await _pumpProfile(tester, service, notesApi: notes);
    await _search(tester, 'b8kp92mqwx7r');

    expect(
      find.text('No encontramos un perfil con ese código.'),
      findsOneWidget,
    );
    expect(find.byType(PublicProfileScreen), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('el buscador rechaza UUID y enlaces web sin consultar', (
    tester,
  ) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService();
    final requests = <String>[];
    final notes = _notesApi(requests: requests);
    addTearDown(service.dispose);
    addTearDown(notes.dispose);

    await _pumpProfile(tester, service, notesApi: notes);
    await _search(tester, '550e8400-e29b-41d4-a716-446655440000');
    expect(
      find.text(publicIdInputMessage(PublicIdInputError.internalUuid)),
      findsOneWidget,
    );
    await _search(tester, 'https://sigma.example/perfil/b8kp92mqwx7r');
    expect(
      find.text(publicIdInputMessage(PublicIdInputError.link)),
      findsOneWidget,
    );
    expect(find.byType(PublicProfileScreen), findsNothing);
    expect(requests, isEmpty);
    expect(tester.takeException(), isNull);
  });
}
