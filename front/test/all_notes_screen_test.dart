import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

void main() {
  // Fixture mínimo para garantizar la resolución real de 'prueba.pdf' en la
  // raíz del paquete aunque el archivo no venga en el checkout.
  final pdfFixture = File('prueba.pdf');
  var createdPdfFixture = false;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

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

  testWidgets('detalle de nota se expande al ancho completo en desktop', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    await tester.tap(find.text('Nota con Adjuntos de Prueba (Conejita y PDF)'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    final sheet = find.byType(DraggableScrollableSheet);
    expect(sheet, findsOneWidget);
    // Sin el tope M3 de 640px, la hoja ocupa el ancho completo de la ventana.
    expect(tester.getSize(sheet).width, 1440);
    expect(find.text('AMPLIAR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('detalle de nota en movil 320dp no produce desbordes', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(320, 800));

    final noteFinder = find.text('Nota con Adjuntos de Prueba (Conejita y PDF)');
    await tester.scrollUntilVisible(
      noteFinder,
      200,
      scrollable: find.byType(Scrollable).last,
    );
    await tester.ensureVisible(noteFinder);
    await tester.pump(const Duration(milliseconds: 200));
    await tester.tap(noteFinder);
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.byType(DraggableScrollableSheet), findsOneWidget);
    expect(find.text('AMPLIAR'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('imagen embebida acota ancho de lectura y encuadra el marco', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    await tester.tap(find.text('Nota con Adjuntos de Prueba (Conejita y PDF)'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));
    // Deja avanzar la decodificación real de la imagen intercalando pumps.
    for (var i = 0; i < 8; i++) {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 50)),
      );
      await tester.pump(const Duration(milliseconds: 100));
    }

    final frame = find.byWidgetPredicate(
      (w) =>
          w is ConstrainedBox &&
          w.constraints.minHeight == 250 &&
          w.constraints.maxHeight == 520 &&
          w.constraints.maxWidth == 850,
    );
    expect(frame, findsOneWidget);

    final size = tester.getSize(frame);
    expect(size.width, lessThanOrEqualTo(850));
    expect(size.height, lessThanOrEqualTo(520));
    expect(size.height, greaterThanOrEqualTo(250));
    expect(tester.takeException(), isNull);
  });

  testWidgets('visor de imagen usa lienzo generoso con pan y zoom', (
    tester,
  ) async {
    await pumpNotes(tester, const Size(1440, 900));

    await tester.tap(find.text('Nota con Adjuntos de Prueba (Conejita y PDF)'));
    await tester.pump(const Duration(milliseconds: 300));
    await tester.pump(const Duration(milliseconds: 300));

    final scrollFinder = find.byType(Scrollable).last;
    await tester.scrollUntilVisible(
      find.text('RECURSOS ADJUNTOS'),
      200,
      scrollable: scrollFinder,
    );
    await tester.pump(const Duration(milliseconds: 200));

    final thumb = find.byWidgetPredicate(
      (w) =>
          w is Container &&
          w.constraints ==
              const BoxConstraints.tightFor(width: 104, height: 78),
    );
    expect(thumb, findsOneWidget);
    await tester.ensureVisible(thumb);
    await tester.pump(const Duration(milliseconds: 200));
    await tester.tap(thumb);
    await tester.pump(const Duration(milliseconds: 300));

    final canvas = find.byWidgetPredicate(
      (w) =>
          w is Container &&
          w.constraints ==
              const BoxConstraints.tightFor(width: 900, height: 480),
    );
    expect(canvas, findsOneWidget);
    expect(tester.getSize(canvas), const Size(900, 480));
    expect(find.byType(InteractiveViewer), findsOneWidget);
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
    await SessionManager.saveSession('jwt-stage2-test', {'id': 'u-stage2'});
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
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
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
    await SessionManager.clear();
    var httpCalls = 0;
    notesHttpClientOverride = MockClient((request) async {
      httpCalls++;
      return http.Response('{}', 500);
    });
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
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
    await SessionManager.saveSession('jwt-stage2-fail', {'id': 'u-stage2'});
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
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
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

