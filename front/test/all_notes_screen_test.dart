import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

void main() {
  // Fixture mínimo para garantizar la resolución real de 'prueba.pdf' en la
  // raíz del paquete aunque el archivo no venga en el checkout.
  final pdfFixture = File('prueba.pdf');
  var createdPdfFixture = false;

  setUpAll(() {
    if (!pdfFixture.existsSync()) {
      pdfFixture.writeAsBytesSync(<int>[
        0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34, 0x0A, // %PDF-1.4
      ]);
      createdPdfFixture = true;
    }
  });

  tearDownAll(() {
    if (createdPdfFixture && pdfFixture.existsSync()) {
      pdfFixture.deleteSync();
    }
  });

  Future<void> pumpNotes(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: AllNotesScreen()));
    // Sin pumpAndSettle: la barra de sincronización es un indicador
    // indeterminado que nunca "asienta". Drift usa I/O real, así que se le da
    // una ventana fuera del reloj falso antes de continuar.
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 300)),
    );
    for (var i = 0; i < 8; i++) {
      await tester.pump(const Duration(milliseconds: 250));
    }
  }

  testWidgets('renderiza y alterna la vista de la grilla en desktop', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
    expect(find.byTooltip('Vista grilla'), findsOneWidget);

    await tester.tap(find.byTooltip('Vista grilla'), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Vista lista'), findsOneWidget);

    await tester.tap(find.byTooltip('Vista lista'), warnIfMissed: false);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.byTooltip('Vista grilla'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza la lista sin desbordes', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(360, 800));

    expect(find.text('TODAS LAS NOTAS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('abre el detalle de la nota y renderiza Markdown y recursos adjuntos', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    final noteFinder = find.text('Nota con Adjuntos de Prueba (Conejita y PDF)');
    expect(noteFinder, findsOneWidget);

    await tester.tap(noteFinder);
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('AMPLIAR'), findsOneWidget);

    final scrollFinder = find.byType(Scrollable).last;
    await tester.scrollUntilVisible(
      find.text('RECURSOS ADJUNTOS'),
      200,
      scrollable: scrollFinder,
    );
    await tester.pump(const Duration(milliseconds: 200));

    expect(find.text('RECURSOS ADJUNTOS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('dialogo de nueva nota incluye barra de adjuntos e inserta preset rapido', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    final newNoteBtn = find.widgetWithText(NeobrutalistButton, 'NUEVA NOTA');
    expect(newNoteBtn, findsOneWidget);

    await tester.tap(newNoteBtn);
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.descendant(of: find.byType(AlertDialog), matching: find.text('NUEVA NOTA')), findsOneWidget);
    expect(find.text('ADJUNTAR:'), findsOneWidget);
    expect(find.text('IMAGEN'), findsOneWidget);
    expect(find.text('PDF / DOC'), findsOneWidget);

    await tester.tap(find.text('IMAGEN'));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('ADJUNTAR IMAGEN'), findsOneWidget);
    expect(find.text('Conejita de prueba (conejita.jpg)'), findsOneWidget);

    await tester.tap(find.text('+ USAR'));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.textContaining('ADJUNTOS VINCULADOS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('parser Markdown renderiza H4 como subtítulo sin almohadillas', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    await tester.tap(find.text('Nota con Adjuntos de Prueba (Conejita y PDF)'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('Verificación técnica H4'), findsOneWidget);
    expect(find.text('#### Verificación técnica H4'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('visor PDF resuelve el archivo real y ofrece apertura externa', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    await tester.tap(find.text('Cálculo - Límites y derivadas'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    final scrollFinder = find.byType(Scrollable).last;
    await tester.scrollUntilVisible(
      find.text('RECURSOS ADJUNTOS'),
      200,
      scrollable: scrollFinder,
    );
    await tester.pump(const Duration(milliseconds: 200));
    await tester.drag(scrollFinder, const Offset(0, -120));
    await tester.pump(const Duration(milliseconds: 200));

    expect(find.text('VER'), findsOneWidget);
    await tester.tap(find.text('VER'));
    await tester.pump(const Duration(milliseconds: 300));

    final dialog = find.byType(AlertDialog);
    expect(dialog, findsOneWidget);
    expect(
      find.descendant(of: dialog, matching: find.textContaining('DOCUMENTO LISTO')),
      findsOneWidget,
    );
    expect(
      find.descendant(of: dialog, matching: find.textContaining('prueba.pdf')),
      findsWidgets,
    );

    final openBtn = find.descendant(
      of: dialog,
      matching: find.textContaining('ABRIR EN VISOR'),
    );
    expect(openBtn, findsOneWidget);

    await tester.tap(openBtn);
    await tester.pump(const Duration(milliseconds: 300));
    expect(find.textContaining('Abriendo'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  // Deja avanzar I/O real (lectura de bytes) y la respuesta HTTP simulada,
  // intercalando pumps para que la UI procese el resultado.
  Future<void> settleRealAsync(WidgetTester tester) async {
    for (var i = 0; i < 8; i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 50)),
      );
      await tester.pump(const Duration(milliseconds: 100));
    }
  }

  Future<void> openAttachmentDialog(
    WidgetTester tester, {
    required String kind,
  }) async {
    await tester.tap(find.widgetWithText(NeobrutalistButton, 'NUEVA NOTA'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.tap(find.text(kind));
    await tester.pump(const Duration(milliseconds: 300));
  }

  testWidgets('adjunto con JWT sube los bytes reales a /notes/upload y usa la URL de Drive', (
    tester,
  ) async {
    SessionManager.saveSession('jwt-stage2-test', {'id': 'u-stage2'});
    final requests = <http.Request>[];
    notesHttpClientOverride = MockClient((request) async {
      requests.add(request);
      if (request.method == 'POST' && request.url.path == '/notes/upload') {
        return http.Response(
          jsonEncode(<String, dynamic>{
            'external_file_id': 'drive_att_1',
            'file_url': 'https://drive.google.com/file/d/drive_att_1/view',
            'file_name': 'prueba.pdf',
            'file_type': 'application/pdf',
            'file_size_bytes': 123,
            'is_inline': false,
          }),
          201,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('{}', 404);
    });
    addTearDown(() {
      notesHttpClientOverride = null;
      SessionManager.clear();
    });

    await pumpNotes(tester, const Size(1440, 900));
    await openAttachmentDialog(tester, kind: 'PDF / DOC');
    expect(find.text('PDF de prueba (prueba.pdf)'), findsOneWidget);

    await tester.tap(find.text('+ USAR'));
    await tester.pump();
    await settleRealAsync(tester);

    expect(find.textContaining('Subido a Google Drive'), findsOneWidget);
    expect(find.textContaining('ADJUNTOS VINCULADOS'), findsOneWidget);
    expect(requests, hasLength(1));
    final upload = requests.single;
    expect(upload.method, 'POST');
    expect(upload.url.path, '/notes/upload');
    expect(upload.headers['Authorization'], 'Bearer jwt-stage2-test');
    expect(upload.body, contains('name="file"'));
    expect(upload.body, contains('filename="prueba.pdf"'));
    expect(tester.takeException(), isNull);
  });

  testWidgets('adjunto sin sesión conserva la referencia local con advertencia amigable', (
    tester,
  ) async {
    SessionManager.clear();
    var httpCalls = 0;
    notesHttpClientOverride = MockClient((request) async {
      httpCalls++;
      return http.Response('{}', 500);
    });
    addTearDown(() {
      notesHttpClientOverride = null;
      SessionManager.clear();
    });

    await pumpNotes(tester, const Size(1440, 900));
    await openAttachmentDialog(tester, kind: 'PDF / DOC');

    await tester.tap(find.text('+ USAR'));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.textContaining('Sin sesión activa'), findsOneWidget);
    expect(find.textContaining('ADJUNTOS VINCULADOS'), findsOneWidget);
    expect(httpCalls, 0);
    expect(tester.takeException(), isNull);
  });

  testWidgets('adjunto con fallo de Drive mantiene fallback local sin crashear', (
    tester,
  ) async {
    SessionManager.saveSession('jwt-stage2-fail', {'id': 'u-stage2'});
    final requests = <http.Request>[];
    notesHttpClientOverride = MockClient((request) async {
      requests.add(request);
      return http.Response(
        jsonEncode(<String, dynamic>{
          'error': {
            'code': 'drive_unavailable',
            'message': 'Drive no disponible',
          },
        }),
        500,
        headers: {'content-type': 'application/json'},
      );
    });
    addTearDown(() {
      notesHttpClientOverride = null;
      SessionManager.clear();
    });

    await pumpNotes(tester, const Size(1440, 900));
    await openAttachmentDialog(tester, kind: 'PDF / DOC');

    await tester.tap(find.text('+ USAR'));
    await tester.pump();
    await settleRealAsync(tester);

    expect(
      find.textContaining('No se pudo subir el adjunto a Drive'),
      findsOneWidget,
    );
    expect(find.textContaining('ADJUNTOS VINCULADOS'), findsOneWidget);
    expect(requests, hasLength(1));
    expect(tester.takeException(), isNull);
  });
}

