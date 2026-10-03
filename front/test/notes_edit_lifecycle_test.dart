import 'dart:convert';
import 'package:drift/native.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/authed_client.dart';
import 'package:taller_integracion_front/core/services/auth_service.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/authenticated_attachment_image.dart';

void main() {
  // JWT determinista para evitar flakiness por exp basado en now():
  // el mock compara el Bearer exacto tras el refresh.
  String jwt(String suffix) =>
      'eyJhbGciOiJIUzI1NiJ9.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.$suffix';
  const id = '12345678-1234-4234-8234-123456789abc';
  const image = 'abcdefab-1234-4234-8234-123456789abc';
  const pdf = 'abcdefab-1234-4234-8234-123456789abd';
  Future<void> settle(WidgetTester t) async {
    await t.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 80)),
    );
    for (var i = 0; i < 5; i++) {
      await t.pump(const Duration(milliseconds: 100));
    }
  }

  for (final local in [false, true]) {
    testWidgets('ediciones consecutivas, cache, JWT y adjuntos local=$local', (
      t,
    ) async {
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession(jwt('first'), {
        'id': 'owner',
      }, refreshToken: 'test-refresh');
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      final repo = LocalNotesRepository(db);
      final noteId = local ? '1234567891234' : id;
      var version = 4, attempts = 0;
      var title = 'Nota de prueba';
      var content =
          'texto antes\n\n![foto](attachment:$image)\n\ntexto después\n\n[guia.pdf](attachment:$pdf)';
      final sent = <int>[];
      final deleted = <String>[];
      final attachments = <Map<String, dynamic>>[
        {
          'id': image,
          'note_id': id,
          'file_type': 'image/png',
          'file_name': 'foto.png',
          'external_file_id': 'img-drive',
        },
        {
          'id': pdf,
          'note_id': id,
          'file_type': 'application/pdf',
          'file_name': 'guia.pdf',
          'external_file_id': 'pdf-drive',
        },
      ];
      await t.runAsync(
        () => repo.upsertNote(
          LocalNote(
            id: noteId,
            title: title,
            content: content,
            visibility: 'private',
            updatedAt: DateTime.now(),
            ownerUserId: 'owner',
            version: local ? null : version,
          ),
        ),
      );
      http.Response json(Object data, [int code = 200]) => http.Response(
        jsonEncode(data),
        code,
        headers: {'content-type': 'application/json; charset=utf-8'},
      );
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'PATCH') {
          expect(local, isFalse, reason: 'Nunca enviar timestamp al backend');
          expect(r.url.path, '/notes/$id');
          attempts++;
          if (attempts == 1)
            return json({
              'error': {'code': 'unauthorized'},
            }, 401);
          expect(r.headers['Authorization'], 'Bearer ${jwt('renewed')}');
          final data = jsonDecode(r.body) as Map<String, dynamic>;
          expect(data['version'], version);
          sent.add(data['version'] as int);
          version++;
          title = data['title'];
          content = data['content'];
          return json({
            'id': id,
            'title': title,
            'version': version,
            'visibility': 'private',
          });
        }
        if (r.method == 'DELETE') {
          deleted.add(r.url.pathSegments.last);
          attachments.removeWhere((a) => a['id'] == r.url.pathSegments.last);
          return http.Response('', 204);
        }
        if (r.url.path == '/notes/me')
          return json({
            'notes': local
                ? []
                : [
                    {'id': id, 'title': title, 'version': version},
                  ],
          });
        if (r.url.path == '/notes/$id')
          return json({
            'id': id,
            'user_id': 'owner',
            'title': title,
            'content': content,
            'version': version,
            'attachments': attachments,
          });
        if (r.url.path.endsWith('/content'))
          return http.Response.bytes(
            base64Decode(
              'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
            ),
            200,
            headers: {'content-type': 'image/png'},
          );
        return json({}, 404);
      });
      AuthedHttp.refreshOverride = (_) async =>
          RefreshResult.success(accessToken: jwt('renewed'));
      addTearDown(() async {
        notesHttpClientOverride = null;
        AuthedHttp.refreshOverride = null;
        await SessionManager.clear();
      });
      t.view.physicalSize = const Size(1400, 1600);
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.resetPhysicalSize);
      addTearDown(t.view.resetDevicePixelRatio);
      await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
      await settle(t);
      await t.tap(find.text('Nota de prueba'));
      await settle(t);
      if (!local) {
        expect(find.byType(AuthenticatedAttachmentImage), findsNWidgets(2));
        final inline = find.byType(AuthenticatedAttachmentImage).first;
        final before = find.text('texto antes', findRichText: true),
            after = find.text('texto después', findRichText: true);
        expect(t.getTopLeft(before).dy, lessThan(t.getTopLeft(inline).dy));
        expect(t.getTopLeft(inline).dy, lessThan(t.getTopLeft(after).dy));
      }
      for (var i = 0; i < 2; i++) {
        await t.scrollUntilVisible(
          find.text('Editar'),
          250,
          scrollable: find.byType(Scrollable).last,
        );
        await t.tap(find.text('Editar'));
        await settle(t);
        final fields = find.byType(TextField);
        await t.enterText(
          find.widgetWithText(TextField, 'Título'),
          'Título ${i + 1}',
        );
        // Search field remains in the underlying page; last field is content.
        await t.enterText(
          fields.last,
          'texto antes\n\n![foto](attachment:$image)\n\ntexto después\n\n[guia.pdf](attachment:$pdf)\n\nedición ${i + 1}',
        );
        await t.ensureVisible(find.text('GUARDAR'));
        await t.tap(find.text('GUARDAR'));
        await settle(t);
        expect(
          find.text('No se pudo guardar la edición en el servidor.'),
          findsNothing,
        );
        expect(find.text('Editar'), findsOneWidget);
      }
      final stored = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(stored!.single.title, 'Título 2');
      expect(stored.single.content, contains('edición 2'));
      expect(stored.single.version, local ? isNull : 6);
      if (!local) {
        expect(sent, [4, 5]);
        // En lectura no se expone QUITAR/X (solo durante la edición).
        expect(find.byKey(ValueKey('remove-$image')), findsNothing);
        expect(find.byKey(ValueKey('remove-$pdf')), findsNothing);
        await t.scrollUntilVisible(
          find.text('Editar'),
          250,
          scrollable: find.byType(Scrollable).last,
        );
        await t.tap(find.text('Editar'));
        await settle(t);
        expect(find.text('ADJUNTOS VINCULADOS (2)'), findsOneWidget);
        // En edición cada adjunto expone su X; no se pulsa, así que nada se
        // borra y CANCELAR conserva adjuntos e inline.
        expect(find.byKey(ValueKey('remove-$image')), findsOneWidget);
        expect(find.byKey(ValueKey('remove-$pdf')), findsOneWidget);
        expect(find.textContaining('SE QUITARÁ'), findsNothing);
        await t.tap(find.text('CANCELAR'));
        await settle(t);
        expect(find.text('Editar'), findsOneWidget);
        // Adjuntos e inline intactos tras el ciclo.
        expect(deleted, isEmpty);
        expect(content, contains('attachment:$image'));
        expect(content, contains('attachment:$pdf'));
        expect(find.byType(AuthenticatedAttachmentImage), findsWidgets);
      } else {
        expect(attempts, 0);
      }
      await t.pumpWidget(const SizedBox());
      await settle(t);
    });
  }

  testWidgets('borrar solo Markdown NO elimina attachment', (t) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwt('first'), {
      'id': 'owner',
    }, refreshToken: 'test-refresh');
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    final repo = LocalNotesRepository(db);
    const singleImage = 'abcdefab-1234-4234-8234-123456789abc';
    var version = 4;
    var content =
        'texto antes\n\n![foto](attachment:$singleImage)\n\ntexto después';
    final attachments = <Map<String, dynamic>>[
      {
        'id': singleImage,
        'note_id': id,
        'file_type': 'image/png',
        'file_name': 'foto.png',
        'external_file_id': 'img-drive',
      },
    ];
    http.Response json(Object data, [int code = 200]) => http.Response(
      jsonEncode(data),
      code,
      headers: {'content-type': 'application/json; charset=utf-8'},
    );
    notesHttpClientOverride = MockClient((r) async {
      if (r.method == 'PATCH') {
        final data = jsonDecode(r.body) as Map<String, dynamic>;
        version++;
        content = data['content'];
        return json({'id': id, 'title': data['title'], 'version': version});
      }
      if (r.method == 'DELETE') {
        attachments.removeWhere((a) => a['id'] == r.url.pathSegments.last);
        return http.Response('', 204);
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': id, 'title': 'Nota de prueba', 'version': version},
          ],
        });
      }
      if (r.url.path == '/notes/$id') {
        return json({
          'id': id,
          'user_id': 'owner',
          'title': 'Nota de prueba',
          'content': content,
          'version': version,
          'attachments': attachments,
        });
      }
      if (r.url.path.endsWith('/content')) {
        return http.Response.bytes(
          base64Decode(
            'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
          ),
          200,
          headers: {'content-type': 'image/png'},
        );
      }
      return json({}, 404);
    });
    AuthedHttp.refreshOverride = (_) async =>
        RefreshResult.success(accessToken: jwt('renewed'));
    addTearDown(() async {
      notesHttpClientOverride = null;
      AuthedHttp.refreshOverride = null;
      await SessionManager.clear();
    });
    await t.runAsync(
      () => repo.upsertNote(
        LocalNote(
          id: id,
          title: 'Nota de prueba',
          content: content,
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: 'owner',
          version: version,
        ),
      ),
    );
    Future<void> settle2(WidgetTester t) async {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 80)),
      );
      for (var i = 0; i < 5; i++) {
        await t.pump(const Duration(milliseconds: 100));
      }
    }

    t.view.physicalSize = const Size(1400, 1600);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await settle2(t);
    await t.tap(find.text('Nota de prueba'));
    await settle2(t);
    // Inline en el cuerpo + miniatura en Recursos Adjuntos.
    expect(find.byType(AuthenticatedAttachmentImage), findsNWidgets(2));
    // Editar: borrar el texto Markdown pero NO pulsar X del adjunto.
    await t.scrollUntilVisible(
      find.text('Editar'),
      250,
      scrollable: find.byType(Scrollable).last,
    );
    await t.tap(find.text('Editar'));
    await settle2(t);
    final fields = find.byType(TextField);
    await t.enterText(fields.last, 'solo texto sin imagen');
    await t.ensureVisible(find.text('GUARDAR'));
    await t.tap(find.text('GUARDAR'));
    await settle2(t);
    // Attachment preservado en servidor aunque el inline desapareció:
    // queda solo la miniatura de Recursos Adjuntos (1 widget).
    expect(attachments.length, 1);
    expect(content, isNot(contains('attachment:$singleImage')));
    expect(find.byType(AuthenticatedAttachmentImage), findsOneWidget);
    // Y en lectura no hay ningún botón quitar.
    expect(find.byKey(ValueKey('remove-$singleImage')), findsNothing);
    await t.pumpWidget(const SizedBox());
    await settle2(t);
  });
}
