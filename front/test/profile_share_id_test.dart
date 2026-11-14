import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/academic/profile_screen.dart';

/// Construye un JWT mínimo con el claim `user_id` que lee SessionManager.
/// La firma no se valida en el cliente: solo se decodifica el payload.
String _tokenWithUserId(String? userId) {
  final payload = userId == null
      ? <String, dynamic>{}
      : <String, dynamic>{'user_id': userId};
  final encoded = base64Url.encode(utf8.encode(jsonEncode(payload)));
  return 'eyJhbGciOiJub25lIn0.$encoded.signature';
}

ProfileService _profileService({String? publicId = 'a8kp92mqwx7r'}) {
  return ProfileService(
    client: MockClient((request) async {
      if (request.url.path.endsWith('/google-drive/status')) {
        return http.Response(
          '{"connected":false}',
          200,
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

Future<void> _pumpProfile(WidgetTester tester, ProfileService service) async {
  tester.view.physicalSize = const Size(1280, 1200);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(MaterialApp(home: ProfileScreen(service: service)));
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

  testWidgets('Compartir perfil copia solo el public_id', (tester) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final clipboardWrites = <String>[];
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments as Map<dynamic, dynamic>;
          clipboardWrites.add(args['text'] as String);
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
    final service = _profileService();
    addTearDown(service.dispose);

    await _pumpProfile(tester, service);
    await tester.tap(find.byKey(const ValueKey('profile-share-id')));
    await tester.pumpAndSettle();

    expect(clipboardWrites, ['a8kp92mqwx7r']);
    expect(find.text('a8kp92mqwx7r'), findsOneWidget);
    expect(find.text('ID de perfil copiado al portapapeles.'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('el buscador rechaza un UUID y no navega', (tester) async {
    await SessionManager.saveSession(_tokenWithUserId('user-abc-123'), {
      'id': 'user-abc-123',
    });
    final service = _profileService();
    addTearDown(service.dispose);

    await _pumpProfile(tester, service);
    await tester.enterText(
      find.byKey(const ValueKey('public-profile-search')),
      '550e8400-e29b-41d4-a716-446655440000',
    );
    await tester.tap(find.byKey(const ValueKey('public-profile-open')));
    await tester.pumpAndSettle();

    expect(
      find.text('Ingresa un ID de perfil válido de 12 caracteres.'),
      findsOneWidget,
    );
    expect(find.byKey(const ValueKey('public-profile-card')), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
