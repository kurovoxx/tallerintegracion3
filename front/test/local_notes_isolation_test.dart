import 'dart:convert';
import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';

String _jwtFor(String userId) {
  String enc(Map<String, dynamic> m) =>
      base64Url.encode(utf8.encode(jsonEncode(m))).replaceAll('=', '');
  final exp =
      DateTime.now().add(const Duration(hours: 1)).millisecondsSinceEpoch ~/
      1000;
  return '${enc({'alg': 'HS256'})}.${enc({'user_id': userId, 'exp': exp})}.firma';
}

LocalNote _note(String id, String? owner) => LocalNote(
  id: id,
  title: 'Nota $id',
  content: 'contenido',
  visibility: 'private',
  updatedAt: DateTime.now(),
  ownerUserId: owner,
);

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('currentUserId lee user_id del JWT sin verificar firma', () async {
    expect(SessionManager.currentUserId, isNull);
    await SessionManager.saveSession(_jwtFor('user-a'), {'id': 'x'});
    expect(SessionManager.currentUserId, 'user-a');
    await SessionManager.clear();
    expect(SessionManager.currentUserId, isNull);
  });

  test('aislamiento: A crea, B no ve, A vuelve y sí ve', () async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    await repo.upsertNote(_note('a1', 'user-a'));
    await repo.upsertNote(_note('a2', 'user-a'));
    await repo.upsertNote(_note('b1', 'user-b'));
    await repo.upsertNote(_note('legacy1', null));

    var a = await repo.getAllNotes(ownerId: 'user-a');
    expect(a.map((n) => n.id).toSet(), {'a1', 'a2'});

    var b = await repo.getAllNotes(ownerId: 'user-b');
    expect(b.map((n) => n.id).toSet(), {'b1'});

    // Legacy sin dueño: visible solo sin sesión, nunca como ajena.
    var anon = await repo.getAllNotes();
    expect(anon.map((n) => n.id).toSet(), {'legacy1'});

    // Búsqueda también filtra por dueño.
    final searchA = await repo.searchNotesFts('Nota', ownerId: 'user-a');
    expect(searchA.map((n) => n.id).toSet(), {'a1', 'a2'});

    // A vuelve y sigue viendo las suyas.
    a = await repo.getAllNotes(ownerId: 'user-a');
    expect(a.map((n) => n.id).toSet(), {'a1', 'a2'});

    // Limpieza selectiva no toca al otro.
    await repo.clearOwner('user-a');
    expect(await repo.getAllNotes(ownerId: 'user-a'), isEmpty);
    expect((await repo.getAllNotes(ownerId: 'user-b')).map((n) => n.id), [
      'b1',
    ]);
  });

  test('persistencia: cerrar y reabrir mantiene notas de A', () async {
    final dir = await Directory.systemTemp.createTemp('iso_restart');
    addTearDown(() async {
      try {
        await dir.delete(recursive: true);
      } catch (_) {}
    });
    final path = '${dir.path}${Platform.pathSeparator}notas.db';

    AppDatabase open() =>
        AppDatabase.forTesting(NativeDatabase.createInBackground(File(path)));

    final db1 = open();
    final repo1 = LocalNotesRepository(db1);
    await repo1.upsertNote(_note('persist1', 'user-a'));
    await db1.close();

    // Reapertura (= restart de app): A sigue viendo su nota.
    final db2 = open();
    addTearDown(db2.close);
    final repo2 = LocalNotesRepository(db2);
    expect((await repo2.getAllNotes(ownerId: 'user-a')).map((n) => n.id), [
      'persist1',
    ]);
    // B no la ve tras el restart.
    expect(await repo2.getAllNotes(ownerId: 'user-b'), isEmpty);
  });

  test('logout/login de A mantiene sus notas (mismo equipo)', () async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);
    await repo.upsertNote(_note('sesion1', 'user-a'));

    await SessionManager.saveSession(_jwtFor('user-a'), {'id': 'x'});
    expect(
      (await repo.getAllNotes(
        ownerId: SessionManager.currentUserId,
      )).map((n) => n.id),
      ['sesion1'],
    );
    await SessionManager.clear();
    await SessionManager.saveSession(_jwtFor('user-a'), {'id': 'x'});
    addTearDown(SessionManager.clear);
    expect(
      (await repo.getAllNotes(
        ownerId: SessionManager.currentUserId,
      )).map((n) => n.id),
      ['sesion1'],
    );
  });

  test('legacy NULL no se auto-asigna ni aparece como propia', () async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);
    await repo.upsertNote(_note('vieja', null));

    await SessionManager.saveSession(_jwtFor('user-nueva'), {'id': 'x'});
    addTearDown(SessionManager.clear);
    // Con sesión: la legacy no aparece (tampoco como ajena editable).
    expect(
      await repo.getAllNotes(ownerId: SessionManager.currentUserId),
      isEmpty,
    );
    // Sin sesión sí es visible como local del equipo.
    await SessionManager.clear();
    expect((await repo.getAllNotes()).map((n) => n.id), ['vieja']);
  });
}
