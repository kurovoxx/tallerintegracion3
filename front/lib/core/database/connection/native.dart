import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';
import 'package:sqlite3_flutter_libs/sqlite3_flutter_libs.dart';

QueryExecutor openConnection() {
  return LazyDatabase(() async {
    try {
      if (Platform.isAndroid || Platform.isIOS) {
        await applyWorkaroundToOpenSqlite3OnOldAndroidVersions();
      }
      final dbFolder = await getApplicationDocumentsDirectory();
      final file = File(p.join(dbFolder.path, 'sigma_academy.db'));
      return NativeDatabase.createInBackground(file, logStatements: false);
    } catch (e) {
      // Fallback en memoria si la inicialización nativa falla (ej. Windows sin sqlite3.dll)
      return NativeDatabase.memory(logStatements: false);
    }
  });
}
