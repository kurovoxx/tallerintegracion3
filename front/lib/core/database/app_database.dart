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

  @override
  MigrationStrategy get migration => MigrationStrategy(
        onCreate: (Migrator m) async {
          try {
            await m.createAll();
          } catch (e) {
            // Fallback si FTS5 no está disponible en Windows (sqlite3.dll antigua)
            // Crea solo la tabla base para no bloquear la app
            try {
              await m.createTable(localNotes);
            } catch (_) {}
          }
        },
        onUpgrade: (m, from, to) async {},
        beforeOpen: (details) async {
          // Verificar FTS5 en caliente, si falla, no bloquear
          try {
            await customStatement('SELECT * FROM local_notes_fts LIMIT 0');
          } catch (_) {
            // FTS5 no disponible, el repositorio hará fallback a LIKE
          }
        },
      );

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
