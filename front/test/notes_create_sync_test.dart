import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:drift/native.dart';
import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/notes/all_notes_screen.dart';
import 'package:taller_integracion_front/features/notes/note_file_picker.dart';

// Carrera crear/abrir: UNA sola apertura basta (sin cerrar/reabrir).
// 7. crear sin adjuntos → una apertura muestra remoto.
// 8/9/10/12. crear con imagen + click durante sync → espera y muestra sola.
// 11. tempId se reemplaza; 12. GET usa el UUID real.
// 13. error remoto deja copia local usable.
// 14. sin polling: GETs acotados.
// 15. cambio de sesión cancela el sync anterior.
void main() {
  const realId = '12345678-1234-4234-8234-123456789abc';
  const attId = 'abcdefab-1234-4234-8234-123456789abc';
  final pngBytes = base64Decode(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
  );

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

  // Espera con el reloj virtual CASI congelado (pumps de 10ms + espera real
  // breve): las transiciones de ruta avanzan, pero los timeouts del cliente
  // (.timeout 8s/30s, virtuales) no pueden dispararse mientras un mock
  // bloquea el pipeline con un Completer; el IO real (Drift) avanza en
  // runAsync. Presupuesto virtual total << 8s en fases bloqueadas.
  Future<void> waitFrozen(WidgetTester t, bool Function() ready) async {
    for (var i = 0; i < 400; i++) {
      await t.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 5)),
      );
      await t.pump(const Duration(milliseconds: 10));
      if (ready()) return;
    }
    expect(ready(), isTrue, reason: 'operación no terminó (reloj congelado)');
  }

  // Backend en memoria controlable: POST puede bloquearse con [postGate].
  MockClient buildMock({
    required List<String> calls,
    required Map<String, Map<String, dynamic>> remote,
    required int Function() getVersion,
    required void Function() bumpVersion,
    Duration postDelay = Duration.zero,
    int postStatus = 201,
  }) {
    http.Response json(Object b, [int status = 200]) => http.Response(
      jsonEncode(b),
      status,
      headers: {'content-type': 'application/json; charset=utf-8'},
    );
    return MockClient((r) async {
      calls.add('${r.method} ${r.url.path}');
      if (r.method == 'POST' && r.url.path == '/notes') {
        // Retraso VIRTUAL: da ventana determinista para tocar la nota con
        // el POST en vuelo sin que los .timeout del cliente (virtuales)
        // puedan dispararse (presupuesto total << 8s).
        if (postDelay > Duration.zero) await Future.delayed(postDelay);
        if (postStatus != 201) return json({}, postStatus);
        final b = jsonDecode(r.body) as Map<String, dynamic>;
        remote[realId] = {
          'id': realId,
          'title': b['title'],
          'user_id': 'owner',
          'version': getVersion(),
          'content': b['content'],
          'attachments': <Map<String, dynamic>>[],
        };
        return json({'note_id': realId, 'version': getVersion()}, 201);
      }
      // Flujo canónico multipart (Development): el binario va directo a
      // POST /notes/{uuid}/attachments. No se accede al body (binario
      // multipart rompe la decodificación UTF-8 del test).
      if (r.method == 'POST' && r.url.path == '/notes/$realId/attachments') {
        final note = remote[realId]!;
        (note['attachments'] as List).add({
          'id': attId,
          'note_id': realId,
          'file_type': 'image/png',
          'file_name': 'foto.png',
          'external_file_id': 'ext-foto',
        });
        return json({'attachment_id': attId}, 201);
      }
      if (r.method == 'PATCH') {
        final b = jsonDecode(r.body) as Map<String, dynamic>;
        bumpVersion();
        final note = remote[realId]!;
        note['content'] = b['content'];
        note['title'] = b['title'];
        note['version'] = getVersion();
        return json({'version': getVersion(), 'title': b['title']});
      }
      if (r.url.path == '/notes/me') {
        return json({
          'notes': [
            for (final n in remote.values)
              {'id': n['id'], 'title': n['title'], 'version': n['version']},
          ],
        });
      }
      if (r.url.path == '/notes/$realId') {
        return json(remote[realId]!);
      }
      if (r.url.path.endsWith('/content')) {
        return http.Response.bytes(
          pngBytes,
          200,
          headers: {'content-type': 'image/png'},
        );
      }
      return json({}, 404);
    });
  }

  Future<void> setup(
    WidgetTester t, {
    required AppDatabase db,
    required MockClient mock,
  }) async {
    SharedPreferences.setMockInitialValues({});
    await SessionManager.saveSession(jwtFor('owner'), {'id': 'owner'});
    notesHttpClientOverride = mock;
    t.view.physicalSize = const Size(1400, 1800);
    t.view.devicePixelRatio = 1;
    addTearDown(t.view.resetPhysicalSize);
    addTearDown(t.view.resetDevicePixelRatio);
    addTearDown(() async {
      notesHttpClientOverride = null;
      NoteFilePicker.pickOverride = null;
      await SessionManager.clear();
    });
    await t.pumpWidget(MaterialApp(home: AllNotesScreen(database: db)));
    await t.pumpAndSettle();
  }

  Future<void> openCreate(WidgetTester t) async {
    final headerBtn = find.widgetWithText(NeobrutalistButton, 'Nueva nota');
    if (headerBtn.evaluate().isNotEmpty) {
      await t.tap(headerBtn);
    } else {
      await t.tap(find.text('CREAR PRIMERA NOTA'));
    }
    await t.pumpAndSettle();
    expect(find.text('NUEVA NOTA'), findsWidgets);
  }

  Future<void> submitCreate(
    WidgetTester t,
    String title,
    String content,
  ) async {
    await t.enterText(find.widgetWithText(TextField, 'Título'), title);
    await t.enterText(
      find.widgetWithText(TextField, 'Contenido Markdown'),
      content,
    );
    await t.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
    await t.pump();
  }

  Future<void> attachImage(WidgetTester t, Uint8List bytes) async {
    // XFile.fromData no conserva el nombre en esta versión de file_selector;
    // se usa un archivo temporal real para que el picker valide nombre real.
    final tmpPath =
        '${Directory.systemTemp.path}/foto_${DateTime.now().microsecondsSinceEpoch}.png';
    // OJO: IO real dentro de testWidgets debe correr en runAsync; un await
    // directo tras pumps se cuelga (event loop virtual vs real).
    await t.runAsync(() => File(tmpPath).writeAsBytes(bytes));
    addTearDown(() {
      try {
        File(tmpPath).deleteSync();
      } catch (_) {}
    });
    NoteFilePicker.pickOverride = (_) async => XFile(tmpPath);
    await t.tap(find.text('IMAGEN'));
    await t.pumpAndSettle();
    expect(find.text('ADJUNTAR IMAGEN'), findsOneWidget);
    await t.tap(find.text('SELECCIONAR IMAGEN'));
    await waitFor(
      t,
      () => find.textContaining('foto_').evaluate().isNotEmpty,
    );
    await t.tap(find.widgetWithText(NeobrutalistButton, 'SUBIR E INSERTAR'));
    await t.pumpAndSettle();
    expect(find.text('ADJUNTAR IMAGEN'), findsNothing);
  }

  testWidgets('7. crear sin adjuntos: una apertura muestra remoto', (t) async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    final remote = <String, Map<String, dynamic>>{};
    final calls = <String>[];
    await setup(
      t,
      db: db,
      mock: buildMock(
        calls: calls,
        remote: remote,
        getVersion: () => version,
        bumpVersion: () => version++,
      ),
    );
    await openCreate(t);
    await submitCreate(t, 'Sync Uno', 'hola remoto');
    await waitFor(t, () => find.text('Sync Uno').evaluate().isNotEmpty);
    // Pipeline terminó: sin badges transitorios.
    await waitFor(
      t,
      () =>
          find.text('SINCRONIZANDO…').evaluate().isEmpty &&
          find.text('SOLO LOCAL').evaluate().isEmpty,
    );
    // Una sola apertura: el GET usa el UUID real, no el tempId.
    await t.tap(find.text('Sync Uno'));
    await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
    expect(find.text('hola remoto'), findsWidgets);
    expect(calls, contains('POST /notes'));
    expect(calls, contains('GET /notes/$realId'));
    expect(
      calls.where((c) => c.startsWith('GET /notes/') && c != 'GET /notes/me'),
      everyElement('GET /notes/$realId'),
    );
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('8/9/10/12/14. click durante sync con imagen: espera y muestra',
      (t) async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    final remote = <String, Map<String, dynamic>>{};
    final calls = <String>[];
    await setup(
      t,
      db: db,
      mock: buildMock(
        calls: calls,
        remote: remote,
        getVersion: () => version,
        bumpVersion: () => version++,
        postDelay: const Duration(seconds: 2),
      ),
    );
    await openCreate(t);
    await t.enterText(find.widgetWithText(TextField, 'Título'), 'Sync Img');
    await t.enterText(
      find.widgetWithText(TextField, 'Contenido Markdown'),
      'nota con foto',
    );
    await attachImage(t, pngBytes);
    await t.tap(find.widgetWithText(NeobrutalistButton, 'GUARDAR'));
    await t.pump();
    await waitFrozen(t, () => find.text('Sync Img').evaluate().isNotEmpty);
    // El botón permanente 'Nueva nota' del header también matchea el texto
    // (uppercase): se espera a que no queden rutas Dialog abiertas.
    await waitFrozen(t, () => find.byType(Dialog).evaluate().isEmpty);
    // 8. Badge visible mientras el POST sigue bloqueado.
    await waitFrozen(
      t,
      () => find.text('SINCRONIZANDO…').evaluate().isNotEmpty,
    );
    // Click durante el sync: muestra espera, no contenido stale.
    await t.tap(find.text('Sync Img'));
    await waitFrozen(
      t,
      () => find.text('Sincronizando nota…').evaluate().isNotEmpty,
    );
    // 9/10. La hoja se abre sola con la imagen, sin segundo tap.
    await waitFrozen(
      t,
      () =>
          find.text('Editar').evaluate().isNotEmpty &&
          find.text('RECURSOS ADJUNTOS').evaluate().isNotEmpty,
    );
    await waitFrozen(t, () => find.text('foto.png').evaluate().isNotEmpty);
    // Multipart canónico: un solo flujo (sin /notes/upload para crear).
    expect(calls, contains('POST /notes/$realId/attachments'));
    expect(
      calls.where((c) => c == 'POST /notes/upload'),
      isEmpty,
    );
    // 12. Todo contra el UUID real; el tempId jamás toca la red.
    expect(
      calls.indexOf('POST /notes'),
      lessThan(calls.indexOf('GET /notes/$realId')),
    );
    final noteGets = calls
        .where(
          (c) =>
              c.startsWith('GET /notes/') &&
              c != 'GET /notes/me' &&
              !c.endsWith('/content'),
        )
        .toList();
    expect(noteGets, isNotEmpty);
    expect(noteGets, everyElement('GET /notes/$realId'));
    // 14. Sin polling: puñado de GETs (pipeline final + carga del detalle).
    final getCount =
        calls.where((c) => c == 'GET /notes/$realId').length;
    expect(getCount, lessThanOrEqualTo(3));
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('13. error remoto deja copia local usable', (t) async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    final remote = <String, Map<String, dynamic>>{};
    final calls = <String>[];
    await setup(
      t,
      db: db,
      mock: buildMock(
        calls: calls,
        remote: remote,
        getVersion: () => version,
        bumpVersion: () => version++,
        postStatus: 500,
      ),
    );
    await openCreate(t);
    await submitCreate(t, 'Solo Local', 'contenido local');
    await waitFor(t, () => find.text('Solo Local').evaluate().isNotEmpty);
    await waitFor(
      t,
      () => find.text('SOLO LOCAL').evaluate().isNotEmpty,
    );
    // Abre igual (sin espera): copia local usable, se puede editar.
    await t.tap(find.text('Solo Local'));
    await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
    expect(find.text('contenido local'), findsWidgets);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('15. cambio de sesión cancela el sync anterior', (t) async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    var version = 1;
    final remote = <String, Map<String, dynamic>>{};
    final calls = <String>[];
    await setup(
      t,
      db: db,
      mock: buildMock(
        calls: calls,
        remote: remote,
        getVersion: () => version,
        bumpVersion: () => version++,
        postDelay: const Duration(seconds: 2),
      ),
    );
    await openCreate(t);
    await submitCreate(t, 'Cambio Sesión', 'texto');
    await waitFrozen(
      t,
      () => find.text('SINCRONIZANDO…').evaluate().isNotEmpty,
    );
    // Otra sesión antes de que responda el POST (retraso virtual 2s).
    await t.runAsync(
      () => SessionManager.saveSession(jwtFor('other'), {'id': 'other'}),
    );
    await t.pumpAndSettle();
    // Sin crash: la nota local sigue ahí y se puede abrir.
    expect(find.text('Cambio Sesión'), findsOneWidget);
    await t.tap(find.text('Cambio Sesión'));
    await waitFor(t, () => find.text('Editar').evaluate().isNotEmpty);
    expect(find.text('texto'), findsWidgets);
    await t.pumpWidget(const SizedBox());
  });
}
