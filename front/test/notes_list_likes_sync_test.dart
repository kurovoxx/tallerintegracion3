import 'support/service_uri.dart';
import 'dart:convert';

import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// "Mis notas" adopta likes_count/is_liked de GET /notes/me. Antes la lista
// arrancaba en 0 y solo se corregía al abrir la nota (GET /stats), así que un
// like dado desde la pestaña de notas del grupo no se veía al volver.
void main() {
  const noteId = '12345678-1234-4234-8234-123456789abc';

  String jwtFor(String userId) =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

  testWidgets('la lista muestra los likes del servidor sin abrir la nota', (
    t,
  ) async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(db.close);
    final calls = <String>[];
    final mock = MockClient((r) async {
      calls.add('${r.method} ${r.url.servicePath}');
      if (r.url.servicePath == '/notes/me') {
        return http.Response(
          jsonEncode({
            'notes': [
              {
                'id': noteId,
                'title': 'Nota likeada en el grupo',
                'version': 1,
                'visibility': 'private',
                'likes_count': 7,
                'is_liked': true,
              },
            ],
            'next_cursor': '',
          }),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('{}', 404);
    });

    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
    notesHttpClientOverride = mock;
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });

    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    for (var i = 0; i < 60; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 20)),
      );
      await t.pump(const Duration(milliseconds: 50));
      if (find.text('7').evaluate().isNotEmpty) break;
    }

    expect(find.text('Nota likeada en el grupo'), findsWidgets);
    // Contador real del servidor en la tarjeta, sin haber abierto la nota.
    expect(find.text('7'), findsWidgets);
    expect(calls.any((c) => c.endsWith('/stats')), isFalse);
  });
}
