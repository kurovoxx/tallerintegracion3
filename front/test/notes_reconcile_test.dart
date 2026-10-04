import 'dart:async';
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
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';

// Reconciliación Drive→App desde REFRESCAR / SINCRONIZAR DRIVE.
// 13. todo correcto → termina rápido con mensaje.
// 14. backend reporta nota eliminada → desaparece de lista.
// 15. backend reporta attachment eliminado → mensaje + refresh.
// 16. error temporal → no borra nada visualmente.
// 17. _isReconciling vuelve a false (implícito en waitIdle de cada test).
// 18. doble click no duplica el reconcile.
void main() {
  const uuid1 = '12345678-1234-4234-8234-123456789abc';
  const uuid2 = '22345678-1234-4234-8234-123456789abc';

  String jwtFor(String userId) =>
      'e30.${base64Url.encode(utf8.encode(jsonEncode({'user_id': userId, 'exp': 9999999999}))).replaceAll('=', '')}.test';

  Future<void> waitFor(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 80; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 30)),
      );
      await t.pump(const Duration(milliseconds: 100));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó');
  }

  Finder syncButton() => find.widgetWithText(
    NeobrutalistButton,
    'REFRESCAR / SINCRONIZAR DRIVE',
  );

  http.Response json(Object b, [int status = 200]) => http.Response(
    jsonEncode(b),
    status,
    headers: {'content-type': 'application/json; charset=utf-8'},
  );

  Future<AppDatabase> seedDb() async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    final repo = LocalNotesRepository(db);
    for (final e in [('Uno', uuid1), ('Dos', uuid2)]) {
      await repo.upsertNote(
        LocalNote(
          id: e.$2,
          title: e.$1,
          content: 'contenido',
          visibility: 'private',
          updatedAt: DateTime.now(),
          ownerUserId: 'owner',
          version: 2,
        ),
      );
    }
    return db;
  }

  Future<void> pumpScreen(WidgetTester t, AppDatabase db) async {
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    addTearDown(() async {
      notesHttpClientOverride = null;
      await SessionManager.clear();
    });
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
  }

  Future<void> tapSync(WidgetTester t) async {
    await t.ensureVisible(syncButton());
    await t.tap(syncButton());
    await t.pump();
  }

  Future<void> waitIdle(WidgetTester t) async {
    await waitFor(t, () => syncButton().evaluate().isNotEmpty);
  }

  // Mock con backend consistente: lo reconciliado desaparece de /notes/me.
  // postGate (opcional): si se provee, el POST espera al test (sin
  // depender del reloj virtual) para probar doble pulsación.
  MockClient buildMock(
    List<String> calls, {
    required Map<String, dynamic> Function() reconcile,
    Completer<void>? postGate,
  }) {
    final gone = <String>{};
    return MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes/reconcile') {
        if (postGate != null) await postGate.future;
        final res = reconcile();
        for (final id in (res['removed_note_ids'] as List?)?.whereType<String>() ?? const <String>[]) {
          gone.add(id);
        }
        return json(res);
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            if (!gone.contains(uuid1))
              {'id': uuid1, 'title': 'Uno', 'version': 2},
            if (!gone.contains(uuid2))
              {'id': uuid2, 'title': 'Dos', 'version': 2},
          ],
        });
      }
      return json({}, 404);
    });
  }

  testWidgets('13. todo correcto termina rápido con mensaje', (t) async {
    final calls = <String>[];
    final db = await seedDb();
    notesHttpClientOverride = buildMock(
      calls,
      reconcile: () => {
        'removed_notes': 0,
        'removed_attachments': 0,
        'pending': 0,
        'removed_note_ids': [],
      },
    );
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(calls, contains('POST /notes/reconcile'));
    expect(calls.where((c) => c.startsWith('PATCH')), isEmpty);
    expect(calls, contains('GET /notes/me'));
    expect(find.text('Tus notas están sincronizadas.'), findsOneWidget);
    expect(find.text('Editar'), findsNothing);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('14. nota eliminada en Drive desaparece de lista', (t) async {
    final calls = <String>[];
    final db = await seedDb();
    notesHttpClientOverride = buildMock(
      calls,
      reconcile: () => {
        'removed_notes': 1,
        'removed_attachments': 1,
        'pending': 0,
        'removed_note_ids': [uuid1],
      },
    );
    await pumpScreen(t, db);
    await waitFor(t, () => find.text('Uno').evaluate().isNotEmpty);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(find.text('Uno'), findsNothing);
    expect(find.text('Dos'), findsOneWidget);
    expect(find.textContaining('eliminados en Drive'), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('15. attachment eliminado: mensaje + refresh sin borrar nota', (
    t,
  ) async {
    final calls = <String>[];
    final db = await seedDb();
    notesHttpClientOverride = buildMock(
      calls,
      reconcile: () => {
        'removed_notes': 0,
        'removed_attachments': 1,
        'pending': 0,
        'removed_note_ids': [],
      },
    );
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(find.text('Uno'), findsOneWidget);
    expect(find.text('Dos'), findsOneWidget);
    expect(find.textContaining('eliminados en Drive'), findsOneWidget);
    expect(calls, contains('GET /notes/me'));
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('16. error temporal no borra nada visualmente', (t) async {
    final calls = <String>[];
    final db = await seedDb();
    notesHttpClientOverride = MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes/reconcile') {
        return json({}, 502);
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            {'id': uuid1, 'title': 'Uno', 'version': 2},
            {'id': uuid2, 'title': 'Dos', 'version': 2},
          ],
        });
      }
      return json({}, 404);
    });
    await pumpScreen(t, db);
    calls.clear();
    await tapSync(t);
    await waitIdle(t);
    expect(find.text('Uno'), findsOneWidget);
    expect(find.text('Dos'), findsOneWidget);
    expect(find.textContaining('Sincronización parcial'), findsOneWidget);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('18. doble click no duplica el reconcile', (t) async {
    final calls = <String>[];
    final db = await seedDb();
    final gate = Completer<void>();
    addTearDown(() {
      if (!gate.isCompleted) gate.complete();
    });
    notesHttpClientOverride = buildMock(
      calls,
      postGate: gate,
      reconcile: () => {
        'removed_notes': 0,
        'removed_attachments': 0,
        'pending': 0,
        'removed_note_ids': [],
      },
    );
    await pumpScreen(t, db);
    calls.clear();
    // Doble invocación directa y consecutiva (sin pumps intermedios que
    // dispersen el dispatch de gestos): la primera arma el guard de forma
    // síncrona y la segunda es no-op. El gate mantiene el sync en curso.
    await t.ensureVisible(syncButton());
    // Doble invocación directa y consecutiva: la primera arma el guard de
    // forma síncrona (_isReconciling=true antes del primer await) y la
    // segunda retorna de inmediato. El gate mantiene el sync en curso.
    final btn = t.widget<NeobrutalistButton>(syncButton());
    btn.onPressed!();
    btn.onPressed!();
    await t.pump();
    gate.complete();
    await waitIdle(t);
    expect(
      calls.where((c) => c == 'POST /notes/reconcile').length,
      1,
    );
    await t.pumpWidget(const SizedBox());
  });
}
