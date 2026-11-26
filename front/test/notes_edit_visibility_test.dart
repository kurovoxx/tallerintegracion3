import 'support/service_uri.dart';
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
import 'package:taller_integracion_front/core/theme/app_theme.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// Cambiar visibilidad al editar una nota propia:
// - el PATCH solo incluye `visibility` cuando cambió;
// - CANCELAR descarta la elección;
// - tras GUARDAR el badge y Drift reflejan la visibilidad del servidor;
// - funciona igual con ambas paletas (Sigma y UCT) y en ancho móvil.
void main() {
  final jwt =
      'eyJhbGciOiJIUzI1NiJ9.${base64Url.encode(utf8.encode(jsonEncode({'user_id': 'owner', 'exp': 9999999999}))).replaceAll('=', '')}.sig';
  const id = '12345678-1234-4234-8234-123456789abc';

  Future<void> settle(WidgetTester t) async {
    await t.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 80)),
    );
    for (var i = 0; i < 5; i++) {
      await t.pump(const Duration(milliseconds: 100));
    }
  }

  http.Response json(Object data, [int code = 200]) => http.Response(
    jsonEncode(data),
    code,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  final cases = <(AppColorMode, Size)>[
    (AppColorMode.neobrutalist, const Size(1400, 1600)),
    (AppColorMode.uct, const Size(1400, 1600)),
    (AppColorMode.neobrutalist, const Size(360, 740)),
    (AppColorMode.uct, const Size(360, 740)),
  ];

  for (final (mode, size) in cases) {
    testWidgets('editar visibilidad privada→pública ${mode.name} '
        '${size.width.toInt()}px', (t) async {
      AppThemeController.mode.value = mode;
      addTearDown(
        () => AppThemeController.mode.value = AppColorMode.neobrutalist,
      );
      SharedPreferences.setMockInitialValues({});
      await SessionManager.saveSession(jwt, {
        'id': 'owner',
      }, refreshToken: 'test-refresh');
      final db = AppDatabase.forTesting(NativeDatabase.memory());
      final repo = LocalNotesRepository(db);
      var version = 3;
      var visibility = 'private';
      const title = 'Nota visibilidad';
      const content = 'contenido';
      final patches = <Map<String, dynamic>>[];
      await t.runAsync(
        () => repo.upsertNote(
          LocalNote(
            needsSync: false,
            id: id,
            title: title,
            content: content,
            visibility: visibility,
            updatedAt: DateTime.now(),
            ownerUserId: 'owner',
            version: version,
          ),
        ),
      );
      notesHttpClientOverride = MockClient((r) async {
        if (r.method == 'PATCH') {
          final data = jsonDecode(r.body) as Map<String, dynamic>;
          patches.add(data);
          version++;
          visibility = (data['visibility'] as String?) ?? visibility;
          return json({
            'id': id,
            'title': data['title'],
            'visibility': visibility,
            'version': version,
          });
        }
        if (r.url.servicePath == '/notes/me') {
          return json({
            'notes': [
              {
                'id': id,
                'title': title,
                'version': version,
                'visibility': visibility,
              },
            ],
          });
        }
        if (r.url.servicePath == '/notes/$id') {
          return json({
            'id': id,
            'user_id': 'owner',
            'title': title,
            'content': content,
            'visibility': visibility,
            'version': version,
            'attachments': [],
          });
        }
        return json({}, 404);
      });
      addTearDown(() async {
        notesHttpClientOverride = null;
        await SessionManager.clear();
      });
      t.view.physicalSize = size;
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.resetPhysicalSize);
      addTearDown(t.view.resetDevicePixelRatio);

      await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
      await settle(t);
      await t.tap(find.text(title).first);
      await settle(t);
      expect(find.text('PRIVADO'), findsWidgets);

      Future<void> openEditor() async {
        await t.scrollUntilVisible(
          find.text('Editar'),
          250,
          scrollable: find.byType(Scrollable).last,
        );
        await t.tap(find.text('Editar'));
        await settle(t);
      }

      Future<void> tapPublic() async {
        final option = find.byKey(const ValueKey('visibility-public'));
        await t.ensureVisible(option);
        await t.tap(option);
        await t.pump();
      }

      // 1. Elegir PÚBLICO y CANCELAR: no se envía nada ni cambia el badge.
      await openEditor();
      expect(find.byKey(const ValueKey('visibility-private')), findsOneWidget);
      expect(find.byKey(const ValueKey('visibility-change-notice')), findsNothing);
      await tapPublic();
      expect(
        find.byKey(const ValueKey('visibility-change-notice')),
        findsOneWidget,
      );
      await t.ensureVisible(find.text('CANCELAR'));
      await t.tap(find.text('CANCELAR'));
      await settle(t);
      expect(patches, isEmpty);
      expect(find.text('PÚBLICO'), findsNothing);

      // 2. Guardar sin tocar visibilidad: el PATCH no la incluye.
      await openEditor();
      await t.ensureVisible(find.text('GUARDAR'));
      await t.tap(find.text('GUARDAR'));
      await settle(t);
      expect(patches, hasLength(1));
      expect(patches.single.containsKey('visibility'), isFalse);

      // 3. Cambiar a PÚBLICO y GUARDAR: PATCH con visibility, badge y Drift.
      await openEditor();
      await tapPublic();
      await t.ensureVisible(find.text('GUARDAR'));
      await t.tap(find.text('GUARDAR'));
      await settle(t);
      expect(patches, hasLength(2));
      expect(patches.last['visibility'], 'public');
      expect(find.text('Editar'), findsOneWidget);
      expect(find.text('PÚBLICO'), findsOneWidget);
      final stored = await t.runAsync(() => repo.getAllNotes(ownerId: 'owner'));
      expect(stored!.single.visibility, 'public');

      // Sin overflows en ninguna paleta / ancho.
      expect(t.takeException(), isNull);
      await t.pumpWidget(const SizedBox());
      await settle(t);
    });
  }
}
