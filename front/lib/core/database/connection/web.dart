import 'package:drift/drift.dart';
import 'package:drift/wasm.dart';
import 'package:sqlite3/wasm.dart';

/// Conexión para Web usando WasmDatabase (IndexedDB / OPFS cuando esté disponible).
///
/// Requiere tener `sqlite3.wasm` y `drift_worker.js` en `web/` (ver https://drift.simonbinder.eu/web/).
/// Estos archivos ya están en `web/` (copiados desde drift 2.34.4) y
/// `flutter build web` los copia automáticamente a `build/web/`.
QueryExecutor openConnection() {
  return LazyDatabase(() async {
    // Wasm in-memory con InMemoryFileSystem es suficiente para `flutter build web`
    // y no requiere worker/OPFS. Si sqlite3.wasm no existe en runtime, el
    // catch lanza error controlado; la app seguirá funcionando salvo notas offline.
    final sqlite3 = await WasmSqlite3.loadFromUrl(Uri.parse('sqlite3.wasm'));
    sqlite3.registerVirtualFileSystem(InMemoryFileSystem(), makeDefault: true);
    return WasmDatabase.inMemory(sqlite3);

    // Alternativa con persistencia (IndexedDB/OPFS) si quieres durabilidad:
    // final result = await WasmDatabase.open(
    //   databaseName: 'sigma_academy',
    //   sqlite3Uri: Uri.parse('sqlite3.wasm'),
    //   driftWorkerUri: Uri.parse('drift_worker.js'),
    // );
    // return result.resolvedExecutor;
  });
}
