import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:file_selector/file_selector.dart';
import 'package:taller_integracion_front/features/notes/note_file_picker.dart';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/attachment_resources.dart';

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

  testWidgets(
    'abre el detalle de la nota y renderiza Markdown y recursos adjuntos',
    (tester) async {
      await pumpNotes(tester, const Size(1440, 900));

      final noteFinder = find.text(
        'Nota con Adjuntos de Prueba (Conejita y PDF)',
      );
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
    },
  );

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

    final noteFinder = find.text(
      'Nota con Adjuntos de Prueba (Conejita y PDF)',
    );
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

  testWidgets(
    'dialogo de adjuntos abre vacío y permite insertar una imagen propia',
    (tester) async {
      await pumpNotes(tester, const Size(1440, 900));

      final newNoteBtn = find.widgetWithText(NeobrutalistButton, 'NUEVA NOTA');
      expect(newNoteBtn, findsOneWidget);

      await tester.tap(newNoteBtn);
      await tester.pump(const Duration(milliseconds: 300));

      expect(
        find.descendant(
          of: find.byType(AlertDialog),
          matching: find.text('NUEVA NOTA'),
        ),
        findsOneWidget,
      );
      expect(find.text('ADJUNTAR:'), findsOneWidget);
      expect(find.text('IMAGEN'), findsOneWidget);
      expect(find.text('PDF'), findsOneWidget);

      await tester.tap(find.text('IMAGEN'));
      await tester.pump(const Duration(milliseconds: 300));

      expect(find.text('ADJUNTAR IMAGEN'), findsOneWidget);
      expect(find.textContaining('PRESET RÁPIDO'), findsNothing);
      expect(find.text('Conejita'), findsNothing);
      await tester.tap(find.text('Usar enlace manual'));
      await tester.pump();
      final imageSource = find.widgetWithText(
        TextField,
        'Ruta local o URL web de imagen',
      );
      expect(tester.widget<TextField>(imageSource).controller!.text, isEmpty);
      await tester.enterText(imageSource, 'https://example.com/diagrama.png');
      await tester.tap(find.text('SUBIR E INSERTAR'));
      await tester.pump(const Duration(milliseconds: 300));

      expect(find.textContaining('ADJUNTOS VINCULADOS'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

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
      find.descendant(
        of: dialog,
        matching: find.textContaining('DOCUMENTO LISTO'),
      ),
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

  testWidgets(
    'detalle de nota usa metadata y bytes privados dentro del Markdown',
    (tester) async {
      const noteId = '12345678-1234-4234-8234-123456789abc';
      const attachmentId = 'abcdefab-1234-4234-8234-123456789abc';
      const driveUrl = 'https://drive.google.com/file/d/private-picture/view';
      final png = base64Decode(
        'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
      );
      final contentPaths = <String>[];
      await SessionManager.saveSession('jwt-private-note', {
        'id': 'private-note-owner',
      });
      notesHttpClientOverride = MockClient((request) async {
        expect(request.headers['Authorization'], 'Bearer jwt-private-note');
        if (request.url.path == '/notes/me') {
          return http.Response(
            jsonEncode({
              'notes': [
                {
                  'id': noteId,
                  'title': 'Imagen privada integrada',
                  'visibility': 'private',
                },
              ],
            }),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.url.path == '/notes/$noteId') {
          return http.Response(
            jsonEncode({
              'content': '![Diagrama]($driveUrl?usp=drivesdk)',
              'attachments': [
                {
                  'id': attachmentId,
                  'note_id': noteId,
                  'file_type': 'image/png',
                  'file_name': 'diagrama.png',
                  'file_url': driveUrl,
                },
              ],
            }),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        if (request.url.path.endsWith('/content')) {
          contentPaths.add(request.url.path);
          return http.Response.bytes(
            png,
            200,
            headers: {'content-type': 'image/png'},
          );
        }
        return http.Response('{}', 404);
      });
      addTearDown(() async {
        notesHttpClientOverride = null;
        await SessionManager.clear();
      });
      await pumpNotes(tester, const Size(1440, 900));
      await settleRealAsync(tester);
      await tester.tap(find.text('Imagen privada integrada'));
      await settleRealAsync(tester);
      expect(contentPaths, isNotEmpty);
      expect(
        contentPaths.every(
          (path) => path == '/notes/$noteId/attachments/$attachmentId/content',
        ),
        isTrue,
      );
      expect(
        find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),
        findsWidgets,
      );
      expect(find.text('DOCUMENTOS'), findsNothing);
      expect(find.text('Imagen no disponible'), findsNothing);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'imagen seleccionada conserva un recurso sin usar Drive HTML como imagen',
    (tester) async {
      final directory = Directory.systemTemp.createTempSync(
        'notes-image-test-',
      );
      final fixture = File('${directory.path}/foto.png');
      fixture.writeAsBytesSync(
        base64Decode(
          'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII=',
        ),
      );
      await SessionManager.saveSession('jwt-image-test', {'id': 'u-image'});
      const url =
          'https://drive.google.com/file/d/private-image/view?usp=drivesdk';
      notesHttpClientOverride = MockClient((request) async {
        if (request.method == 'POST' && request.url.path == '/notes/upload') {
          return http.Response(
            jsonEncode({
              'external_file_id': 'private-image',
              'file_url': url,
              'file_name': 'foto.png',
              'file_type': 'image/png',
              'file_size_bytes': fixture.lengthSync(),
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
        fixture.deleteSync();
        directory.deleteSync();
      });
      await pumpNotes(tester, const Size(1440, 900));
      await openAttachmentDialog(tester, kind: 'IMAGEN');
      NoteFilePicker.pickOverride = (_) async => XFile.fromData(fixture.readAsBytesSync(),path:fixture.path);
      addTearDown(() => NoteFilePicker.pickOverride = null);
      await tester.tap(find.text('SELECCIONAR IMAGEN'));
      await settleRealAsync(tester);
      await tester.tap(find.text('SUBIR E INSERTAR'));
      await settleRealAsync(tester);
      expect(find.text('ADJUNTOS VINCULADOS (1)'), findsOneWidget);
      final markdown = tester
          .widgetList<TextField>(find.byType(TextField))
          .map((field) => field.controller?.text ?? '')
          .firstWhere((value) => value.contains('attachment-pending://'));
      expect(resourcesFromMarkdown(markdown).single['type'], 'image');
      await tester.tap(find.text('foto'));
      await tester.pump(const Duration(milliseconds: 300));

      expect(find.textContaining('Archivo preparado'), findsWidgets);
      expect(
        find.byWidgetPredicate((w) => w is Image && w.image is NetworkImage),
        findsNothing,
      );
      expect(find.text('DOCUMENTOS'), findsNothing);
      expect(tester.takeException(), isNull);
    },
  );

  for (final scenario in [(false, false), (true, false), (false, true)]) {
    final fail = scenario.$1;
    final refresh = scenario.$2;
    testWidgets('archivo nuevo espera UUID: fallo=$fail refresh=$refresh', (
      tester,
    ) async {
      await SessionManager.saveSession('jwt-new', {
        'id': 'u-new',
      }, refreshToken: 'refresh-test');
      var uploads = 0;
      AuthedHttp.refreshOverride = (_) async =>
          RefreshResult.success(accessToken: 'jwt-renovado');
      const id = '12345678-1234-4234-8234-123456789abc';
      final paths = <String>[];
      var created = false;
      notesHttpClientOverride = MockClient((r) async {
        paths.add('${r.method} ${r.url.path}');
        if (r.method == 'POST' && r.url.path == '/notes') {
          created = true;
          return http.Response('{"note_id":"$id"}', 201);
        }
        if (r.url.path == '/notes/upload') {
          uploads++;
          if (refresh && uploads == 1) {
            return http.Response('{"error":"unauthorized"}', 401);
          }
          expect(created, isTrue);
          expect(r.body, contains('filename="guia.pdf"'));
          if (fail) {
            return http.Response('{"error":{"code":"drive_unavailable"}}', 500);
          }
          return http.Response(
            jsonEncode({
              'external_file_id': 'file1',
              'file_url': 'https://drive.google.com/file/d/file1/view',
              'file_name': 'guia.pdf',
              'file_type': 'application/pdf',
              'file_size_bytes': 8,
            }),
            201,
          );
        }
        if (r.url.path == '/notes/$id/attachments') {
          expect(created, isTrue);
          return http.Response('{"attachment_id":"attachment1"}', 201);
        }
        if (r.method == 'PATCH') return http.Response('{}', 200);
        return http.Response('{"notes":[]}', 200);
      });
      NoteFilePicker.pickOverride = (_) async => XFile.fromData(
        Uint8List.fromList([37, 80, 68, 70, 45, 49, 46, 52]),
        path: 'guia.pdf',
      );
      addTearDown(() async {
        notesHttpClientOverride = null;
        NoteFilePicker.pickOverride = null;
        AuthedHttp.refreshOverride = null;
        await SessionManager.clear();
      });
      await pumpNotes(tester, const Size(1440, 1000));
      await openAttachmentDialog(tester, kind: 'PDF');
      await tester.tap(find.text('SELECCIONAR PDF'));
      await settleRealAsync(tester);
      expect(find.textContaining('guia.pdf'), findsOneWidget);
      expect(find.textContaining('C:\\Users'), findsNothing);
      await tester.tap(find.text('SUBIR Y ADJUNTAR'));
      await tester.pump();
      expect(paths.where((p) => p.contains('/upload')), isEmpty);
      expect(find.textContaining('Archivo preparado'), findsOneWidget);
      final title = find.widgetWithText(TextField, 'Título');
      await tester.enterText(title, 'Nota con archivo');
      await tester.tap(find.text('GUARDAR'));
      await settleRealAsync(tester);
      expect(uploads, refresh ? 2 : 1);
      if (!fail) expect(paths, contains('POST /notes/$id/attachments'));
      if (fail) {
        expect(
          find.textContaining('no se pudo adjuntar guia.pdf'),
          findsOneWidget,
        );
      } else {
        expect(paths, contains('PATCH /notes/$id'));
      }
      expect(
        paths.where((p) => RegExp(r'/notes/\d{13}/attachments').hasMatch(p)),
        isEmpty,
      );
      expect(tester.takeException(), isNull);
    });
  }
}
