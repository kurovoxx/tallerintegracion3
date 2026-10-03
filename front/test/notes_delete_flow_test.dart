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
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';

void main() {
  const note = '12345678-1234-4234-8234-123456789abc';
  const image = 'abcdefab-1234-4234-8234-123456789abc';
  const pdf = 'abcdefab-1234-4234-8234-123456789abd';
  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 60; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 30)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  for (final failure in ['none', 'patch', 'attachment', 'note']) {
    testWidgets('borrado Notes, Cancelar/Guardar y error=$failure', (t) async {
      SharedPreferences.setMockInitialValues({});
      final jwt =
          'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.test';
      await SessionManager.saveSession(jwt, {'id': 'owner'});
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      final repo = LocalNotesRepository(db);
      var version = 1, exists = true;
      var content =
          'antes\n\n![foto](attachment:$image)\n\ndespués\n\n[guia.pdf](attachment:$pdf)';
      final atts = <Map<String, dynamic>>[
        {
          'id': image,
          'note_id': note,
          'file_type': 'image/png',
          'file_name': 'foto.png',
          'external_file_id': 'img',
        },
        {
          'id': pdf,
          'note_id': note,
          'file_type': 'application/pdf',
          'file_name': 'guia.pdf',
          'external_file_id': 'pdf',
        },
      ];
      final calls = <String>[];
      http.Response json(Object b, [int status = 200]) => http.Response(
        jsonEncode(b),
        status,
        headers: {'content-type': 'application/json; charset=utf-8'},
      );
      notesHttpClientOverride = MockClient((r) async {
        calls.add('${r.method} ${r.url.path}');
        if (r.method == 'PATCH') {
          if (failure == 'patch') {
            return json({
              'error': {'code': 'conflict'},
            }, 409);
          }
          final b = jsonDecode(r.body);
          expect(b['version'], version);
          version++;
          content = b['content'];
          return json({'version': version, 'title': 'Prueba borrado'});
        }
        if (r.method == 'DELETE') {
          if (r.url.path == '/notes/$note') {
            if (failure == 'note') return json({}, 500);
            exists = false;
            atts.clear();
            return http.Response('', 204);
          }
          if (failure == 'attachment') return json({}, 500);
          final id = r.url.pathSegments.last;
          expect(id, anyOf(image, pdf));
          atts.removeWhere((a) => a['id'] == id);
          return http.Response('', 204);
        }
        if (r.url.path == '/notes/me') {
          return json({
            'notes': exists
                ? [
                    {'id': note, 'title': 'Prueba borrado', 'version': version},
                  ]
                : [],
          });
        }
        if (r.url.path == '/notes/$note') {
          return json({
            'id': note,
            'title': 'Prueba borrado',
            'user_id': 'owner',
            'version': version,
            'content': content,
            'attachments': atts,
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
      addTearDown(() async {
        notesHttpClientOverride = null;
        await SessionManager.clear();
      });
      t.view.physicalSize = const Size(1400, 1800);
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.resetPhysicalSize);
      addTearDown(t.view.resetDevicePixelRatio);
      await t.pumpWidget(
        MaterialApp(
          home: AllNotesScreen(database: db, enableAttachmentRemoval: true),
        ),
      );
      await waitFor(t, () => find.text('Prueba borrado').evaluate().isNotEmpty);
      await t.tap(find.text('Prueba borrado'));
      await waitFor(
        t,
        () => find.text('RECURSOS ADJUNTOS').evaluate().isNotEmpty,
      );
      Future<void> edit() async {
        await t.scrollUntilVisible(
          find.text('Editar'),
          250,
          scrollable: find.byType(Scrollable).last,
        );
        await t.tap(find.text('Editar'));
        await t.pumpAndSettle();
      }

      Future<void> mark(String id) async {
        final f = find.byKey(ValueKey('remove-$id'));
        await t.ensureVisible(f);
        await t.tap(f);
        await t.pump();
      }

      await edit();
      await mark(image);
      expect(calls.where((c) => c.startsWith('DELETE')), isEmpty);
      await t.ensureVisible(find.text('CANCELAR'));
      await t.tap(find.text('CANCELAR'));
      await t.pumpAndSettle();
      expect(atts.length, 2);
      expect(content, contains('attachment:$image'));
      await edit();
      await mark(image);
      await t.ensureVisible(find.text('GUARDAR'));
      await t.tap(find.text('GUARDAR'));
      await waitFor(t, () => calls.any((c) => c.startsWith('PATCH')));
      if (failure == 'patch') {
        await waitFor(
          t,
          () => find.textContaining('otra sesión').evaluate().isNotEmpty,
        );
        expect(calls.where((c) => c.startsWith('DELETE')), isEmpty);
        expect(atts.length, 2);
        await t.tap(find.text('CANCELAR'));
        await t.pumpAndSettle();
      } else {
        await waitFor(
          t,
          () =>
              find.text('Editar').evaluate().isNotEmpty &&
              calls.where((c) => c == 'GET /notes/$note').length >= 2,
        );
        expect(content, isNot(contains('attachment:$image')));
        expect(content, contains('attachment:$pdf'));
        expect(atts.length, failure == 'attachment' ? 2 : 1);
        expect(
          calls.indexOf('PATCH /notes/$note'),
          lessThan(calls.indexOf('DELETE /notes/$note/attachments/$image')),
        );
        await edit();
        expect(
          find.byKey(const ValueKey('remove-$image')),
          failure == 'attachment' ? findsOneWidget : findsNothing,
        );
        expect(find.byKey(const ValueKey('remove-$pdf')), findsOneWidget);
        if (failure == 'none') {
          await mark(pdf);
          await t.ensureVisible(find.text('GUARDAR'));
          await t.tap(find.text('GUARDAR'));
          await waitFor(
            t,
            () =>
                atts.isEmpty &&
                calls.where((c) => c == 'GET /notes/$note').length >= 2 &&
                find.text('Editar').evaluate().isNotEmpty,
          );
          expect(content, isNot(contains('attachment:')));
          expect(find.text('RECURSOS ADJUNTOS'), findsNothing);
        } else {
          await t.ensureVisible(find.text('CANCELAR'));
          await t.tap(find.text('CANCELAR'));
          await t.pumpAndSettle();
        }
      }
      await t.scrollUntilVisible(
        find.text('Eliminar'),
        250,
        scrollable: find.byType(Scrollable).last,
      );
      await t.tap(find.text('Eliminar'));
      await t.pumpAndSettle();
      expect(
        find.text(
          '¿Eliminar esta nota? También se eliminarán sus archivos adjuntos.',
        ),
        findsOneWidget,
      );
      await t.tap(find.widgetWithText(NeobrutalistButton, 'CANCELAR'));
      await t.pumpAndSettle();
      expect(exists, isTrue);
      await t.tap(find.text('Eliminar'));
      await t.pumpAndSettle();
      await t.tap(find.widgetWithText(NeobrutalistButton, 'ELIMINAR'));
      await waitFor(t, () => calls.contains('DELETE /notes/$note'));
      if (failure == 'note') {
        await waitFor(
          t,
          () => find
              .text('No se pudo eliminar. Inténtalo más tarde.')
              .evaluate()
              .isNotEmpty,
        );
        expect(exists, isTrue);
        expect(
          (await t.runAsync(() => repo.getAllNotes(ownerId: 'owner')))!.length,
          1,
        );
      } else {
        await waitFor(
          t,
          () => find.byType(DraggableScrollableSheet).evaluate().isEmpty,
        );
        expect(exists, isFalse);
        expect(find.text('Prueba borrado'), findsNothing);
        expect(
          await t.runAsync(() => repo.getAllNotes(ownerId: 'owner')),
          isEmpty,
        );
      }
      await t.pumpWidget(const SizedBox());
    });
  }
}
