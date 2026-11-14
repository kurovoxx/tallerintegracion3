import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/attachment_resources.dart';

import 'support/service_uri.dart';

void main() {
  const noteId = '12345678-1234-4234-8234-123456789abc';
  const attachmentId = 'abcdefab-1234-4234-8234-123456789abc';
  final pdfBytes = Uint8List.fromList(
    utf8.encode('%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF'),
  );

  final pdfResource = attachmentResources(noteId, [
    {
      'id': attachmentId,
      'note_id': noteId,
      // Igual que una copia real: metadata del adjunto clonado con file_url de
      // Drive (mock o real). El visor debe preferir el endpoint autenticado.
      'file_url': 'https://drive.google.com/file/d/att_mock/view',
      'file_name': 'guia.pdf',
      'file_type': 'application/pdf',
      'external_file_id': 'att_mock',
    },
  ]).single;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-notes', {'id': 'reader'});
  });

  tearDown(() async {
    await SessionManager.clear();
  });

  Future<void> pumpViewer(
    WidgetTester tester, {
    required http.Client client,
    Future<bool> Function(Uint8List, String, String)? openAttachment,
    Future<String?> Function(Uint8List, String, String)? saveAttachment,
  }) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: debugBuildResourceViewer(
            pdfResource,
            client: client,
            openAttachment: openAttachment,
            saveAttachment: saveAttachment,
          ),
        ),
      ),
    );
  }

  testWidgets('PDF adjunto se abre con bytes del endpoint autenticado', (
    tester,
  ) async {
    final paths = <String>[];
    final client = MockClient((request) async {
      paths.add(request.url.servicePath);
      expect(request.headers['Authorization'], 'Bearer jwt-notes');
      return http.Response.bytes(
        pdfBytes,
        200,
        headers: {'content-type': 'application/pdf'},
      );
    });
    addTearDown(client.close);
    Uint8List? openedBytes;
    String? openedName;
    String? openedMime;

    await pumpViewer(
      tester,
      client: client,
      openAttachment: (bytes, name, mime) async {
        openedBytes = bytes;
        openedName = name;
        openedMime = mime;
        return true;
      },
    );

    expect(find.text('DOCUMENTO ADJUNTO'), findsOneWidget);
    expect(find.text('ARCHIVO NO ENCONTRADO'), findsNothing);
    expect(find.text('ABRIR EN DRIVE'), findsNothing);
    expect(find.text('ABRIR PDF'), findsOneWidget);

    await tester.tap(find.text('ABRIR PDF'));
    await tester.pumpAndSettle();

    expect(paths, ['/notes/$noteId/attachments/$attachmentId/content']);
    expect(openedBytes, pdfBytes);
    expect(openedName, 'guia.pdf');
    expect(openedMime, 'application/pdf');
    expect(find.textContaining('Abriendo guia.pdf'), findsOneWidget);
  });

  testWidgets('DESCARGAR entrega los mismos bytes y reporta la ruta', (
    tester,
  ) async {
    final client = MockClient(
      (_) async => http.Response.bytes(
        pdfBytes,
        200,
        headers: {'content-type': 'application/pdf'},
      ),
    );
    addTearDown(client.close);
    Uint8List? savedBytes;
    String? savedName;

    await pumpViewer(
      tester,
      client: client,
      saveAttachment: (bytes, name, mime) async {
        savedBytes = bytes;
        savedName = name;
        return '/descargas/guia.pdf';
      },
    );

    await tester.tap(find.text('DESCARGAR'));
    await tester.pumpAndSettle();

    expect(savedBytes, pdfBytes);
    expect(savedName, 'guia.pdf');
    expect(find.textContaining('/descargas/guia.pdf'), findsOneWidget);
  });

  testWidgets('un 404 muestra error accesible y no llama al abridor', (
    tester,
  ) async {
    final client = MockClient((_) async => http.Response('private error', 404));
    addTearDown(client.close);
    var opened = false;

    await pumpViewer(
      tester,
      client: client,
      openAttachment: (_, _, _) async {
        opened = true;
        return true;
      },
    );

    await tester.tap(find.text('ABRIR PDF'));
    await tester.pumpAndSettle();

    expect(opened, isFalse);
    expect(find.text('No tienes acceso a este adjunto.'), findsOneWidget);
    expect(find.text('private error'), findsNothing);
  });
}
