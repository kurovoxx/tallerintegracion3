import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/attachment_content.dart';

import 'support/service_uri.dart';

void main() {
  const noteId = '12345678-1234-4234-8234-123456789abc';
  const attachmentId = 'abcdefab-1234-4234-8234-123456789abc';
  final pdfBytes = Uint8List.fromList(
    utf8.encode('%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n%%EOF'),
  );

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('jwt-notes', {'id': 'reader'});
  });

  tearDown(() async {
    await SessionManager.clear();
  });

  test('descarga el PDF por el endpoint autenticado con Bearer', () async {
    final auth = <String?>[];
    final client = MockClient((request) async {
      expect(
        request.url.servicePath,
        '/notes/$noteId/attachments/$attachmentId/content',
      );
      auth.add(request.headers['Authorization']);
      return http.Response.bytes(
        pdfBytes,
        200,
        headers: {'content-type': 'application/pdf; charset=binary'},
      );
    });
    addTearDown(client.close);

    final content = await fetchAttachmentContent(
      baseUrl: 'https://sigma.test/api-notes',
      noteId: noteId,
      attachmentId: attachmentId,
      fileName: 'guia.pdf',
      client: client,
    );

    expect(auth, ['Bearer jwt-notes']);
    expect(content.mimeType, 'application/pdf');
    expect(content.isPdf, isTrue);
    expect(content.bytes, pdfBytes);
    expect(content.fileName, 'guia.pdf');
  });

  test('un 404 no filtra bytes ni detalles internos', () async {
    final client = MockClient((_) async => http.Response('private error', 404));
    addTearDown(client.close);

    await expectLater(
      fetchAttachmentContent(
        baseUrl: 'https://sigma.test/api-notes',
        noteId: noteId,
        attachmentId: attachmentId,
        fileName: 'guia.pdf',
        client: client,
      ),
      throwsA(
        isA<AttachmentContentException>()
            .having((e) => e.statusCode, 'statusCode', 404)
            .having(
              (e) => e.message,
              'message',
              'No tienes acceso a este adjunto.',
            ),
      ),
    );
  });

  test('UUID inválido se rechaza sin hacer peticiones', () async {
    var requests = 0;
    final client = MockClient((_) async {
      requests++;
      return http.Response('', 200);
    });
    addTearDown(client.close);

    await expectLater(
      fetchAttachmentContent(
        baseUrl: 'https://sigma.test/api-notes',
        noteId: 'not-a-uuid',
        attachmentId: attachmentId,
        fileName: 'guia.pdf',
        client: client,
      ),
      throwsA(isA<AttachmentContentException>()),
    );
    expect(requests, 0);
  });

  test('el nombre descargado no conserva rutas ni sale sin extensión', () {
    final name = safeAttachmentFileName(
      r'..\..\Windows\guia',
      mimeType: 'application/pdf',
    );
    expect(name.contains('/'), isFalse);
    expect(name.contains(r'\'), isFalse);
    expect(name.endsWith('.pdf'), isTrue);
    expect(
      safeAttachmentFileName('   ', mimeType: 'application/pdf'),
      'adjunto.pdf',
    );
    expect(safeAttachmentFileName('foto.png'), 'foto.png');
  });
}
