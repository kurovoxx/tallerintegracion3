import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/testing.dart';
import 'package:http/http.dart' as http;
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

Future<void> _pumpNotes(WidgetTester tester) async {
  tester.view.physicalSize = const Size(1440, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(const MaterialApp(home: AllNotesScreen()));
  await tester.runAsync(
    () => Future<void>.delayed(const Duration(milliseconds: 300)),
  );
  for (var i = 0; i < 8; i++) {
    await tester.pump(const Duration(milliseconds: 250));
  }
}

void main() {
  testWidgets(
      'con sesión y fallo GET /notes/me no se presentan demos como reales',
      (tester) async {
    SessionManager.saveSession('jwt-notes-fail', {'id': 'u-notes'});
    notesHttpClientOverride = MockClient((request) async {
      if (request.method == 'GET' && request.url.path == '/notes/me') {
        return http.Response('{"error":{"code":"internal"}}', 500,
            headers: {'content-type': 'application/json'});
      }
      return http.Response('{}', 404);
    });
    addTearDown(() {
      notesHttpClientOverride = null;
      SessionManager.clear();
    });

    await _pumpNotes(tester);

    // Aviso real de backend, nunca ejemplo como real.
    expect(find.textContaining('No se pudo cargar tus notas (GET /notes/me)'),
        findsOneWidget);
    expect(
        find.text(
            'Modo local sin sesión: se muestran datos de ejemplo, no son tus notas reales. Inicia sesión para GET /notes/me.'),
        findsNothing);
    // Demos verificables (id+título) ocultas en este modo.
    expect(find.text('Cálculo - Límites y derivadas'), findsNothing);
    expect(find.text('Redes - Modelo OSI'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
