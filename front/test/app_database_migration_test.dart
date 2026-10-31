import 'dart:typed_data';
import 'dart:io';

import 'package:drift/drift.dart' show Value;
import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sqlite3/sqlite3.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';

LocalNote _note(String id, String? owner, {int? version}) => LocalNote(
  needsSync: false,
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

  // 3_2_18: una base real de la versión 4 (con nota temporal e intención de
  // creación pendiente) migra a v5 sin perder datos: needs_sync se agrega con
  // false y aparece la tabla de adjuntos pendientes.
  test('migración v4->v5 conserva notas, intención y tombstones', () async {
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
        owner_user_id TEXT NULL,
        create_key TEXT NULL,
        create_snapshot TEXT NULL
      );
    ''');
    raw.execute('''
      CREATE TABLE note_tombstones (
        id TEXT NOT NULL PRIMARY KEY,
        owner_user_id TEXT NULL,
        deleted_at INTEGER NOT NULL,
        remote_pending INTEGER NOT NULL DEFAULT 1,
        last_error TEXT NULL
      );
    ''');
    raw.execute(
      'INSERT INTO local_notes VALUES (?, ?, ?, ?, ?, NULL, ?, ?, ?)',
      [
        '1700000000000',
        'Pendiente',
        'cuerpo [a](attachment-pending://a.pdf)',
        'private',
        1700000000,
        'u1',
        'flutter-note-create-1700000000000',
        '{"title":"Pendiente"}',
      ],
    );
    raw.execute('INSERT INTO note_tombstones VALUES (?, ?, ?, 1, NULL)', [
      'borrada',
      'u1',
      1700000000,
    ]);
    raw.execute('PRAGMA user_version = 4');
    raw.close();

    final db = AppDatabase.forTesting(
      NativeDatabase.createInBackground(File(path)),
    );
    addTearDown(db.close);
    final repo = LocalNotesRepository(db);

    final notes = await repo.getAllNotes(ownerId: 'u1');
    expect(notes.single.id, '1700000000000');
    expect(notes.single.needsSync, isFalse);
    expect(notes.single.createKey, 'flutter-note-create-1700000000000');
    expect(notes.single.content, contains('attachment-pending://a.pdf'));
    expect((await repo.getTombstones(ownerId: 'u1')).single.id, 'borrada');

    // La tabla nueva funciona sobre la base migrada.
    await repo.savePendingAttachment(
      placeholder: 'attachment-pending://a.pdf',
      noteId: '1700000000000',
      ownerId: 'u1',
      fileName: 'a.pdf',
      mime: 'application/pdf',
      bytes: Uint8List.fromList([37, 80, 68, 70]),
    );
    expect(
      (await repo.pendingAttachmentsForNote('1700000000000')).single.fileName,
      'a.pdf',
    );
    final version = await db.customSelect('PRAGMA user_version').getSingle();
    expect(version.read<int>('user_version'), 5);
  });

  group('adjuntos pendientes (integridad)', () {
    Future<LocalNotesRepository> open() async {
      final path = await _tempDbPath();
      final db = AppDatabase.forTesting(
        NativeDatabase.createInBackground(File(path)),
      );
      addTearDown(db.close);
      return LocalNotesRepository(db);
    }

    Uint8List bytes(int n) => Uint8List(n);

    test('borrar una nota elimina solo SUS adjuntos pendientes', () async {
      final repo = await open();
      await repo.upsertNote(_note('n1', 'u1'));
      await repo.upsertNote(_note('n2', 'u1'));
      for (final (id, ph) in [('n1', 'p1'), ('n1', 'p2'), ('n2', 'p3')]) {
        await repo.savePendingAttachment(
          placeholder: 'attachment-pending://$ph',
          noteId: id,
          ownerId: 'u1',
          fileName: '$ph.png',
          mime: 'image/png',
          bytes: bytes(8),
        );
      }
      await repo.deleteNote('n1');
      expect(await repo.pendingAttachmentsForNote('n1'), isEmpty);
      expect(
        (await repo.pendingAttachmentsForNote('n2')).single.placeholder,
        'attachment-pending://p3',
      );
      expect((await repo.getAllNotes(ownerId: 'u1')).single.id, 'n2');
    });

    test('promover temp->UUID re-asocia los adjuntos sin huérfanos', () async {
      final repo = await open();
      await repo.upsertNote(
        _note('1700000000000', 'u1').copyWith(
          createKey: const Value('k'),
          createSnapshot: const Value('{"title":"Nota 1700000000000"}'),
        ),
      );
      await repo.savePendingAttachment(
        placeholder: 'attachment-pending://x',
        noteId: '1700000000000',
        ownerId: 'u1',
        fileName: 'x.pdf',
        mime: 'application/pdf',
        bytes: bytes(4),
      );
      const real = '12345678-1234-4234-8234-123456789abc';
      await repo.promoteTempNote('1700000000000', real, version: 1);
      expect(await repo.getNoteById('1700000000000'), isNull);
      expect(await repo.pendingAttachmentsForNote('1700000000000'), isEmpty);
      expect(
        (await repo.pendingAttachmentsForNote(real)).single.fileName,
        'x.pdf',
      );
    });

    test('aislamiento: otra cuenta no lee bytes ni consume cuota', () async {
      final repo = await open();
      await repo.savePendingAttachment(
        placeholder: 'attachment-pending://privado',
        noteId: 'n1',
        ownerId: 'u1',
        fileName: 'privado.pdf',
        mime: 'application/pdf',
        bytes: bytes(16),
      );
      expect(
        await repo.pendingAttachment('attachment-pending://privado', 'u2'),
        isNull,
      );
      expect(
        await repo.pendingAttachment('attachment-pending://privado', null),
        isNull,
      );
      expect(await repo.pendingAttachmentBytes('u1'), 16);
      expect(await repo.pendingAttachmentBytes('u2'), 0);
      await repo.clearOwner('u2');
      expect(await repo.pendingAttachmentBytes('u1'), 16);
      await repo.clearOwner('u1');
      expect(await repo.pendingAttachmentBytes('u1'), 0);
    });

    test('rechaza archivos > 10 MB y respeta la cuota por cuenta', () async {
      final repo = await open();
      await expectLater(
        repo.savePendingAttachment(
          placeholder: 'attachment-pending://grande',
          noteId: 'n1',
          ownerId: 'u1',
          fileName: 'grande.pdf',
          mime: 'application/pdf',
          bytes: bytes(LocalNotesRepository.maxPendingAttachmentBytes + 1),
        ),
        throwsArgumentError,
      );
      expect(
        await repo.canStorePendingAttachments('u1', [
          LocalNotesRepository.maxPendingAttachmentBytes + 1,
        ]),
        isFalse,
      );
      expect(
        await repo.canStorePendingAttachments('u1', [
          for (var i = 0; i < 11; i++)
            LocalNotesRepository.maxPendingAttachmentBytes,
        ]),
        isFalse,
        reason: '110 MB supera la cuota de 100 MB',
      );
      expect(await repo.canStorePendingAttachments('u1', [1024]), isTrue);
      expect(await repo.pendingAttachmentBytes('u1'), 0);
    });
  });
}
