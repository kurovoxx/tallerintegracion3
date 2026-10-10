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

void main() {
  for (final scenario in [
    (200, false),
    (204, false),
    (204, true),
    (500, false),
  ]) {
    final (status, closedCache) = scenario;
    testWidgets('delete status=$status closedCache=$closedCache refreshes list', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final payload = base64Url
          .encode(
            utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999})),
          )
          .replaceAll('=', '');
      await SessionManager.saveSession('eyJhbGciOiJIUzI1NiJ9.$payload.test', {
        'id': 'owner',
      });
      tester.view.physicalSize = const Size(1440, 1000);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      final repo = LocalNotesRepository(db);
      const id = '12345678-1234-4234-8234-123456789abc';
      const siblingId = '12345678-1234-4234-8234-123456789abd';
      final notes = [
        {'id': id, 'title': 'Nota para eliminar', 'content': 'Texto'},
        {'id': siblingId, 'title': 'Nota conservada', 'content': 'Texto'},
      ];
      await tester.runAsync(() async {
        for (final note in notes) {
          await repo.upsertNote(
            LocalNote(
              id: note['id']!,
              title: note['title']!,
              content: note['content']!,
              visibility: 'private',
              updatedAt: DateTime.now(),
              ownerUserId: 'owner',
            ),
          );
        }
      });
      var deletes = 0;
      notesHttpClientOverride = MockClient((request) async {
        if (request.method == 'DELETE') {
          expect(request.url.path, '/notes/$id');
          deletes++;
          return http.Response('', status);
        }
        if (request.url.path == '/notes/me') {
          return http.Response(jsonEncode({'notes': notes}), 200);
        }
        return http.Response(
          jsonEncode({'content': 'Texto', 'version': 1}),
          200,
        );
      });
      addTearDown(() async {
        notesHttpClientOverride = null;
        await SessionManager.clear();
        if (!closedCache) await db.close();
      });

      Future<void> settle() async {
        await tester.runAsync(
          () => Future<void>.delayed(const Duration(milliseconds: 100)),
        );
        for (var i = 0; i < 5; i++) {
          await tester.pump(const Duration(milliseconds: 100));
        }
      }

      await tester.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
      await settle();
      await tester.tap(find.text('Nota para eliminar'));
      await settle();
      if (closedCache) await tester.runAsync(db.close);
      await tester.ensureVisible(find.text('Eliminar'));
      await tester.tap(find.text('Eliminar'));
      await settle();
      await tester.tap(find.text('ELIMINAR').last);
      await settle();
      expect(deletes, 1);
      if (status == 500) {
        expect(find.text('Nota para eliminar'), findsWidgets);
        expect(
          find.text('No se pudo eliminar. Inténtalo más tarde.'),
          findsOneWidget,
        );
      } else {
        expect(find.text('Nota para eliminar'), findsNothing);
        expect(find.text('Nota conservada'), findsOneWidget);
        if (!closedCache) {
          final cached = await tester.runAsync(
            () => repo.getAllNotes(ownerId: 'owner'),
          );
          expect(cached!.map((note) => note.id), [siblingId]);
        }
        // Refiltering stale cache data must not resurrect a server-deleted note.
        await tester.tap(find.byTooltip('Vista grilla'));
        await tester.pump();
        expect(find.text('Nota para eliminar'), findsNothing);
      }
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
    });
  }
}
