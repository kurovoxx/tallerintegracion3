import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';
import 'package:flutter/foundation.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:sqlite3_flutter_libs/sqlite3_flutter_libs.dart';

/// Ruta alternativa para pruebas E2E aisladas:
/// `--dart-define=APP_DB_PATH=C:/ruta/ti3_e2e.db`.
/// Vacío => Documents/sigma_academy.db (comportamiento normal).
const _dbPathOverride = String.fromEnvironment('APP_DB_PATH');

QueryExecutor openConnection() {
  return LazyDatabase(() async {
    try {
      // En `flutter test` no hay plugin de path_provider: usar memoria evita
      // dejar futures de plataforma pendientes que bloquean el cierre de la BD.
      if (!kIsWeb && Platform.environment['FLUTTER_TEST'] == 'true') {
        return NativeDatabase.memory(logStatements: false);
      }
      if (Platform.isAndroid || Platform.isIOS) {
        await applyWorkaroundToOpenSqlite3OnOldAndroidVersions();
      }
      final override = _dbPathOverride.trim();
      final File file;
      if (override.isNotEmpty) {
        file = File(override);
      } else {
        final dbFolder = await getApplicationDocumentsDirectory();
        file = File(p.join(dbFolder.path, 'sigma_academy.db'));
      }
      await file.parent.create(recursive: true);
      debugPrint('[DB] sqlite path=${file.path}');
      return NativeDatabase.createInBackground(file, logStatements: false);
    } catch (e) {
      // Fallback en memoria si la inicialización nativa falla (ej. Windows sin sqlite3.dll)
      debugPrint('[DB] fallback a memoria: $e');
      return NativeDatabase.memory(logStatements: false);
    }
  });
}
