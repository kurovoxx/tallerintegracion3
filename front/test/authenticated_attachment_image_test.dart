import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/features/notes/authenticated_attachment_image.dart';
import 'package:taller_integracion_front/features/notes/attachment_resources.dart';

void main() {
  final png = base64Decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=');
  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession('expired', {'id':'reader'}, refreshToken: 'refresh');
  });
  tearDown(() async { AuthedHttp.refreshOverride = null; await SessionManager.clear(); });

  testWidgets('imagen privada descarga con JWT y reintenta tras refresh', (tester) async {
    final auth = <String?>[];
    var refreshes = 0;
    AuthedHttp.refreshOverride = (_) async {
      refreshes++;
      return RefreshResult.success(accessToken:'renewed');
    };
    final client = MockClient((request) async {
      expect(request.url.path, '/notes/n/attachments/a/content');
      auth.add(request.headers['Authorization']);
      if (auth.length == 1) return http.Response('{}',401);
      return http.Response.bytes(png,200,headers:{'content-type':'image/png'});
    });
    addTearDown(client.close);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: AuthenticatedAttachmentImage(
      contentUri: Uri.parse('https://sigma.test/notes/n/attachments/a/content'),
      fit: BoxFit.contain, onOpenDrive: () {}, client:client,
    ))));
    await tester.pumpAndSettle();
    expect(auth,['Bearer expired','Bearer renewed']);
    expect(refreshes,1);
    expect(find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),findsOneWidget);
    expect(find.byWidgetPredicate((w) => w is Image && w.image is NetworkImage),findsNothing);
    expect(find.text('Imagen no disponible'),findsNothing);
  });
  for (final status in [403,404]) {
    testWidgets('respuesta $status muestra fallback sin filtrar contenido', (tester) async {
      var opened = false;
      final client = MockClient((_) async => http.Response('private error',status));
      addTearDown(client.close);
      await tester.pumpWidget(MaterialApp(home: Scaffold(body: AuthenticatedAttachmentImage(
        contentUri: Uri.parse('https://sigma.test/notes/n/attachments/a/content'),
        fit: BoxFit.contain, onOpenDrive: () { opened = true; }, client:client,
      ))));
      await tester.pumpAndSettle();
      expect(find.text('Imagen no disponible'), findsOneWidget);
      expect(find.text('private error'), findsNothing);
      await tester.tap(find.text('Abrir en Drive'));
      expect(opened,isTrue);
    });
  }
  testWidgets('nota local sin external_file_id sirve bytes locales sin red', (tester) async {
    var requests = 0;
    final client = MockClient((_) async { requests++; return http.Response('{}',404); });
    addTearDown(client.close);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: AuthenticatedAttachmentImage(
      contentUri: Uri.parse('https://sigma.test/notes/n/attachments/a/content'),
      fit: BoxFit.contain, onOpenDrive: () {}, client:client,
      externalFileId:'', localBytes:png,
    ))));
    await tester.pumpAndSettle();
    expect(requests,0);
    expect(find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),findsOneWidget);
    expect(find.text('Imagen no disponible'),findsNothing);
  });
  testWidgets('endpoint caido con copia local cae a memoria sin error', (tester) async {
    final client = MockClient((_) async => http.Response('private error',404));
    addTearDown(client.close);
    await tester.pumpWidget(MaterialApp(home: Scaffold(body: AuthenticatedAttachmentImage(
      contentUri: Uri.parse('https://sigma.test/notes/n/attachments/a/content'),
      fit: BoxFit.contain, onOpenDrive: () {}, client:client,
      externalFileId:'drive-file', localBytes:png,
    ))));
    await tester.pumpAndSettle();
    expect(find.byWidgetPredicate((w) => w is Image && w.image is MemoryImage),findsOneWidget);
    expect(find.text('Imagen no disponible'),findsNothing);
  });
  test('metadata prevalece sobre is_inline y distingue PDF de imagen', () {
    final resources = attachmentResources('note', [
      {'id':'image','note_id':'note','file_type':'image/jpeg','is_inline':false,'file_name':'foto.jpg'},
      {'id':'pdf','note_id':'note','file_type':'application/pdf','is_inline':true,'file_name':'guia.pdf'},
      {'id':'other','note_id':'other-note','file_type':'image/png'},
    ]);
    expect(resources.map((r) => r['type']), ['image','pdf']);
    expect(resources.first['attachment_id'],'image');
    expect(resourceIdentity('https://drive.google.com/file/d/file/view?usp=drivesdk'),
      resourceIdentity('https://drive.google.com/file/d/file/view'));
  });
}
