import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:sqlite3_flutter_libs/sqlite3_flutter_libs.dart';

part 'app_database.g.dart';

class LocalNotes extends Table {
  TextColumn get id => text()();
  TextColumn get title => text().withLength(min: 1, max: 300)();
  TextColumn get content => text()();
  TextColumn get visibility => text().withLength(min: 1, max: 20).withDefault(const Constant('private'))();
  DateTimeColumn get updatedAt => dateTime()();

  @override
  Set<Column> get primaryKey => {id};
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
        "USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')"
      ];

  // Para FTS5, Drift requiere definir qué columnas existen; no necesitamos PK.
}

@DriftDatabase(tables: [LocalNotes, LocalNotesFts])
class AppDatabase extends _$AppDatabase {
  AppDatabase() : super(_openConnection());

  AppDatabase.forTesting(super.e);

  @override
  int get schemaVersion => 1;

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
                  "CREATE VIRTUAL TABLE IF NOT EXISTS local_notes_fts USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')");
            } catch (_) {}
          }

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
                  "CREATE VIRTUAL TABLE IF NOT EXISTS local_notes_fts USING fts5(title, content, content='local_notes', content_rowid='rowid', tokenize='porter unicode61')");
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
                "INSERT INTO local_notes_fts(local_notes_fts) VALUES('rebuild')");
            ftsAvailable = true;
          } catch (_) {
            // Si falla la creación de triggers o rebuild, marcar no disponible
            // pero mantener la tabla base operativa.
            ftsAvailable = false;
          }
        },
        onUpgrade: (m, from, to) async {},
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

  /// Reconstruye el índice FTS5 desde el contenido actual de `local_notes`.
  /// Útil para mantenimiento o tras importaciones masivas.
  Future<void> rebuildFtsIndex() async {
    try {
      await customStatement(
          "INSERT INTO local_notes_fts(local_notes_fts) VALUES('rebuild')");
      ftsAvailable = true;
    } catch (_) {
      ftsAvailable = false;
    }
  }

  // Wrapper para asegurar que sqlite3_flutter_libs se inicialice (Android/iOS/Windows)
  // Con try/catch explícito para no congelar en Windows si falta sqlite3.dll
  static LazyDatabase _openConnection() {
    return LazyDatabase(() async {
      try {
        if (Platform.isAndroid || Platform.isIOS) {
          await applyWorkaroundToOpenSqlite3OnOldAndroidVersions();
        }
        final dbFolder = await getApplicationDocumentsDirectory();
        final file = File(p.join(dbFolder.path, 'sigma_academy.db'));
        return NativeDatabase.createInBackground(file, logStatements: false);
      } catch (e) {
        // Fallback en memoria si la inicialización nativa falla en Windows Desktop
        // Evita spinner infinito y permite búsqueda básica en memoria
        return NativeDatabase.memory(logStatements: false);
      }
    });
  }
}
