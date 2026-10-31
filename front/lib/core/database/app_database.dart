import 'package:drift/drift.dart';

import 'connection/stub.dart'
    if (dart.library.io) 'connection/native.dart'
    if (dart.library.html) 'connection/web.dart'
    as connection;

part 'app_database.g.dart';

class LocalNotes extends Table {
  TextColumn get id => text()();
  TextColumn get title => text().withLength(min: 1, max: 300)();
  TextColumn get content => text()();
  TextColumn get visibility => text()
      .withLength(min: 1, max: 20)
      .withDefault(const Constant('private'))();
  DateTimeColumn get updatedAt => dateTime()();
  // ownerUserId aisla notas por cuenta (ver §10 informe): NULL = fila legacy
  // o creada sin sesión (visible solo sin sesión, nunca como ajena).
  IntColumn get version => integer().nullable()();
  TextColumn get ownerUserId => text().nullable()();
  // Fase 1 (creación idempotente): clave de idempotencia y snapshot del payload
  // original de la PRIMERA intención de creación. Se persisten junto a la nota
  // para reutilizarse tras reiniciar y evitar duplicados en PG/Drive.
  TextColumn get createKey => text().nullable()();
  TextColumn get createSnapshot => text().nullable()();
  BoolColumn get needsSync => boolean().withDefault(const Constant(false))();

  @override
  Set<Column> get primaryKey => {id};
}

// Tombstones de borrado persistentes (Fase 1 §P3): una nota borrada no debe
// reaparecer si el backend/Drive aún la devuelve en un sync posterior.
@DataClassName('NoteTombstoneEntry')
class NoteTombstones extends Table {
  // id de la nota (temporal epoch-ms o UUID remoto).
  TextColumn get id => text()();
  TextColumn get ownerUserId => text().nullable()();
  DateTimeColumn get deletedAt => dateTime()();
  // true = el borrado remoto aún no se confirmó (reintentable). false = el
  // borrado remoto ya obtuvo 200/204/404; se conserva como defensa contra
  // resurrecciones hasta que el sync deje de devolver el id.
  BoolColumn get remotePending => boolean().withDefault(const Constant(true))();
  TextColumn get lastError => text().nullable()();

  @override
  Set<Column> get primaryKey => {id};
}

// Bytes seleccionados antes de crear remoto. El recibo uploadedId evita volver
// a subir si el PATCH del Markdown falla o la app se reinicia después del upload.
class PendingNoteAttachments extends Table {
  TextColumn get placeholder => text()();
  TextColumn get noteId => text()();
  TextColumn get ownerUserId => text().nullable()();
  TextColumn get fileName => text()();
  TextColumn get mime => text()();
  BlobColumn get bytes => blob()();
  TextColumn get uploadedId => text().nullable()();

  @override
  Set<Column> get primaryKey => {placeholder};
}

// Tabla virtual FTS5 para búsqueda offline instantánea por título y contenido.
// Soporta MATCH por palabras clave y prefijos (query*).
// Usa content='local_notes' y content_rowid='rowid' implícitos de Drift no: mejor externa con triggers manuales.
// Simplificado: tabla FTS5 externa que indexa title y content, sincronizada vía triggers.
@DataClassName('LocalNoteFtsEntry')
class LocalNotesFts extends Table {
  TextColumn get title => text()();
  TextColumn get content => text()();

  @override
  String get tableName => 'local_notes_fts';

  @override
  // ignore: override_on_non_overriding_member
  bool get isVirtual => true;

  @override
  // ignore: override_on_non_overriding_member
  List<String> get customConstraints => const [
    "USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')",
  ];

  // Para FTS5, Drift requiere definir qué columnas existen; no necesitamos PK.
}

@DriftDatabase(
  tables: [LocalNotes, LocalNotesFts, NoteTombstones, PendingNoteAttachments],
)
class AppDatabase extends _$AppDatabase {
  AppDatabase() : super(_openConnection());

  AppDatabase.forTesting(super.e);

  @override
  int get schemaVersion => 5;

  /// Bandera de disponibilidad del motor FTS5.
  /// true si la tabla virtual y sus triggers se crearon correctamente
  /// y el binario SQLite soporta FTS5; false para usar fallback LIKE.
  bool ftsAvailable = false;

  @override
  MigrationStrategy get migration => MigrationStrategy(
    onCreate: (Migrator m) async {
      // Intentar creación via drift; en drift 2.34+ la generación de
      // tablas virtuales via Dart puede fallar (warning Could not parse),
      // por lo que hacemos fallback manual robusto.
      try {
        await m.createAll();
      } catch (e) {
        // Fallback si FTS5 no está disponible o drift falló al generar
        // CREATE VIRTUAL TABLE (syntax error). Crear base manualmente.
        try {
          await m.createTable(localNotes);
        } catch (_) {}
        // Crear virtual FTS5 manualmente de forma robusta
        try {
          await customStatement(
            "CREATE VIRTUAL TABLE IF NOT EXISTS local_notes_fts USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')",
          );
        } catch (_) {}
      }

      // Tabla de tombstones (Fase 1 §P3): se garantiza su creación aunque el
      // createAll() haya fallado por el problema de la tabla virtual FTS5.
      await _createTableIfMissing(
        'note_tombstones',
        () => m.createTable(noteTombstones),
      );
      await _createTableIfMissing(
        'pending_note_attachments',
        () => m.createTable(pendingNoteAttachments),
      );

      // Verificar que local_notes_fts sea realmente una tabla virtual FTS5
      // y no una tabla regular mal generada por drift.
      // Si la verificación falla, recrear como virtual.
      try {
        await customStatement('SELECT * FROM local_notes_fts LIMIT 0');
      } catch (_) {
        try {
          await customStatement('DROP TABLE IF EXISTS local_notes_fts');
        } catch (_) {}
        try {
          await customStatement(
            "CREATE VIRTUAL TABLE IF NOT EXISTS local_notes_fts USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')",
          );
        } catch (_) {
          ftsAvailable = false;
          return;
        }
      }

      // Crear triggers de sincronización para tabla FTS5 de contenido externo.
      // Sin estos triggers el índice permanece vacío (bug silencioso).
      try {
        await customStatement('''
CREATE TRIGGER local_notes_ai AFTER INSERT ON local_notes BEGIN
  INSERT INTO local_notes_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END
''');
        await customStatement('''
CREATE TRIGGER local_notes_ad AFTER DELETE ON local_notes BEGIN
  INSERT INTO local_notes_fts(local_notes_fts, rowid, title, content) VALUES('delete', old.rowid, old.title, old.content);
END
''');
        await customStatement('''
CREATE TRIGGER local_notes_au AFTER UPDATE ON local_notes BEGIN
  INSERT INTO local_notes_fts(local_notes_fts, rowid, title, content) VALUES('delete', old.rowid, old.title, old.content);
  INSERT INTO local_notes_fts(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END
''');
        await customStatement(
          "INSERT INTO local_notes_fts(local_notes_fts) VALUES('rebuild')",
        );
        ftsAvailable = true;
      } catch (_) {
        // Si falla la creación de triggers o rebuild, marcar no disponible
        // pero mantener la tabla base operativa.
        ftsAvailable = false;
      }
    },
    onUpgrade: (m, from, to) async {
      // Migraciones idempotentes: el archivo local puede venir de otra
      // rama/versión que ya traía la columna (o de un ALTER previo que sí
      // corrió pero dejó user_version atrás). Ejecutar ADD COLUMN a ciegas
      // lanzaba "duplicate column name: version" y rompía la apertura de la
      // base local. Ahora se verifica con PRAGMA table_info antes.
      if (from < 3) {
        await _addColumnIfMissing(
          m,
          'local_notes',
          'version',
          () => m.addColumn(localNotes, localNotes.version),
        );
      }
      // v1 -> v2: aislamiento por cuenta sin borrar datos legacy.
      if (from < 2) {
        await _addColumnIfMissing(
          m,
          'local_notes',
          'owner_user_id',
          () => m.addColumn(localNotes, localNotes.ownerUserId),
        );
      }
      // v3 -> v4 (Fase 1): idempotencia de creación (clave + snapshot) y
      // tombstones persistentes de borrado.
      if (from < 4) {
        await _addColumnIfMissing(
          m,
          'local_notes',
          'create_key',
          () => m.addColumn(localNotes, localNotes.createKey),
        );
        await _addColumnIfMissing(
          m,
          'local_notes',
          'create_snapshot',
          () => m.addColumn(localNotes, localNotes.createSnapshot),
        );
        await _createTableIfMissing(
          'note_tombstones',
          () => m.createTable(noteTombstones),
        );
      }
      if (from < 5) {
        await _addColumnIfMissing(
          m,
          'local_notes',
          'needs_sync',
          () => m.addColumn(localNotes, localNotes.needsSync),
        );
        await _createTableIfMissing(
          'pending_note_attachments',
          () => m.createTable(pendingNoteAttachments),
        );
      }
    },
    beforeOpen: (details) async {
      // Verificar FTS5 en caliente, si falla, no bloquear
      try {
        await customStatement('SELECT * FROM local_notes_fts LIMIT 0');
        ftsAvailable = true;
      } catch (_) {
        ftsAvailable = false;
      }
    },
  );

  /// true si [table] existe en el esquema local (sqlite_master).
  Future<bool> _hasTable(String table) async {
    final rows = await customSelect(
      "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?",
      variables: [Variable.withString(table)],
    ).get();
    return rows.isNotEmpty;
  }

  /// Ejecuta la creación de [table] solo si aún no existe. Tolera archivos
  /// locales donde la tabla ya fue creada por otra rama/versión.
  Future<void> _createTableIfMissing(
    String table,
    Future<void> Function() create,
  ) async {
    if (await _hasTable(table)) return;
    try {
      await create();
    } catch (_) {
      if (!await _hasTable(table)) rethrow;
    }
  }

  /// true si [table] ya tiene la columna [column] (PRAGMA table_info).
  /// Permite que las migraciones de columna sean tolerantes a archivos que
  /// ya venían con la columna creada por otra rama/versión.
  Future<bool> _hasColumn(String table, String column) async {
    final rows = await customSelect('PRAGMA table_info($table)').get();
    return rows.any((row) => row.read<String>('name') == column);
  }

  /// Ejecuta el ADD COLUMN de [add] solo si la columna aún no existe.
  /// Doble red de seguridad: si el PRAGMA dijera que no existe pero la
  /// columna sí está (carrera/formato raro), se tolera el error
  /// "duplicate column name" y se re-verifica antes de propagar.
  Future<void> _addColumnIfMissing(
    Migrator m,
    String table,
    String column,
    Future<void> Function() add,
  ) async {
    if (await _hasColumn(table, column)) return;
    try {
      await add();
    } catch (_) {
      if (!await _hasColumn(table, column)) rethrow;
    }
  }

  /// Reconstruye el índice FTS5 desde el contenido actual de `local_notes`.
  /// Útil para mantenimiento o tras importaciones masivas.
  Future<void> rebuildFtsIndex() async {
    try {
      await customStatement(
        "INSERT INTO local_notes_fts(local_notes_fts) VALUES('rebuild')",
      );
      ftsAvailable = true;
    } catch (_) {
      ftsAvailable = false;
    }
  }

  // Delega a implementación por plataforma (native.dart / web.dart / stub.dart)
  static QueryExecutor _openConnection() => connection.openConnection();
}
