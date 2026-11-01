import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/profile_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

void main() {
  test('una respuesta anterior no repuebla el perfil de otra sesión', () async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('A', {'id': 'A'});
    final response = Completer<http.Response>();
    final service = ProfileService(client: MockClient((_) => response.future));
    final request = service.getProfile();
    final assertion = expectLater(request, throwsA(isA<Exception>()));
    await SessionManager.clear();
    await SessionManager.saveSession('B', {'id': 'B'});
    response.complete(http.Response('{"display_name":"A"}', 200));
    await assertion;
    expect(ProfileService.current.value, isNull);
    service.dispose();
    await SessionManager.clear();
  });
}
