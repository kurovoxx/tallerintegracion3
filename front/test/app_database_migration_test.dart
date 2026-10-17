import 'dart:io';

import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sqlite3/sqlite3.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';

LocalNote _note(String id, String? owner, {int? version}) => LocalNote(
  id: id,
  title: 'Nota $id',
  content: 'contenido',
  visibility: 'private',
  updatedAt: DateTime.now(),
  ownerUserId: owner,
  version: version,
);

Future<String> _tempDbPath() async {
  final dir = await Directory.systemTemp.createTemp('mig_notes');
  addTearDown(() async {
    try {
      await dir.delete(recursive: true);
    } catch (_) {}
  });
  return '${dir.path}${Platform.pathSeparator}notes.db';
}

void main() {
  // Regresión: el archivo local podía traer la columna `version` ya creada
  // (otra rama/versión) pero con user_version=2, y el ADD COLUMN a ciegas
  // lanzaba "duplicate column name: version" rompiendo la apertura.
  test('migración v2->v3 tolera la columna version ya existente', () async {
    final path = await _tempDbPath();

    final raw = sqlite3.open(path);
    raw.execute('''
      CREATE TABLE local_notes (
        id TEXT NOT NULL PRIMARY KEY,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        visibility TEXT NOT NULL DEFAULT 'private',
        updated_at INTEGER NOT NULL,
        version INTEGER NULL,
        owner_user_id TEXT NULL
      );
    ''');
    raw.execute('PRAGMA user_version = 2');
    raw.close();

    final db = AppDatabase.forTesting(
      NativeDatabase.createInBackground(File(path)),
    );
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    // No debe lanzar: la migración es idempotente.
    await repo.upsertNote(_note('n1', 'u1', version: 3));
    final notes = await repo.getAllNotes(ownerId: 'u1');
    expect(notes.map((n) => n.id), ['n1']);
  });

  // Caso real del archivo local: user_version=1 pero con AMBAS columnas ya
  // creadas. Antes rompía con "duplicate column name"; ahora se omiten.
  test('migración v1 con columnas ya presentes (caso real) no rompe', () async {
    final path = await _tempDbPath();

    final raw = sqlite3.open(path);
    raw.execute('''
      CREATE TABLE local_notes (
        id TEXT NOT NULL PRIMARY KEY,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        visibility TEXT NOT NULL DEFAULT 'private',
        updated_at INTEGER NOT NULL,
        owner_user_id TEXT NULL,
        version INTEGER NULL
      );
    ''');
    raw.execute('PRAGMA user_version = 1');
    raw.close();

    final db = AppDatabase.forTesting(
      NativeDatabase.createInBackground(File(path)),
    );
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    await repo.upsertNote(_note('real', 'u-real', version: 7));
    expect((await repo.getAllNotes(ownerId: 'u-real')).map((n) => n.id), [
      'real',
    ]);
  });

  // Camino normal v1->v3: columnas ausentes se agregan igual.
  test('migración v1->v3 agrega columnas faltantes', () async {
    final path = await _tempDbPath();

    final raw = sqlite3.open(path);
    raw.execute('''
      CREATE TABLE local_notes (
        id TEXT NOT NULL PRIMARY KEY,
        title TEXT NOT NULL,
        content TEXT NOT NULL,
        visibility TEXT NOT NULL DEFAULT 'private',
        updated_at INTEGER NOT NULL
      );
    ''');
    raw.execute('PRAGMA user_version = 1');
    raw.close();

    final db = AppDatabase.forTesting(
      NativeDatabase.createInBackground(File(path)),
    );
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    await repo.upsertNote(_note('n2', 'u2', version: 5));
    final notes = await repo.getAllNotes(ownerId: 'u2');
    expect(notes.map((n) => n.id), ['n2']);
    expect(notes.single.version, 5);
  });

  test('base nueva (onCreate) abre y persiste sin migración', () async {
    final db = AppDatabase.forTesting(NativeDatabase.memory());
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    await repo.upsertNote(_note('fresh', 'u3'));
    expect((await repo.getAllNotes(ownerId: 'u3')).map((n) => n.id), ['fresh']);
  });
}
