import 'package:flutter/foundation.dart';

import 'app_database.dart';

/// Dueño único de la [AppDatabase] de la app.
///
/// Las pantallas piden una instancia con [acquire] y avisan con [release] en
/// `dispose`, pero **no cierran** la base: el cierre es responsabilidad de este
/// manager (al salir de la app) o de [resetForTesting] en tests.
///
/// Motivo: `NativeDatabase.createInBackground` ejecuta SQLite en un isolate.
/// Cerrar la conexión desde el `dispose` de una pantalla mientras una
/// sincronización sigue en vuelo produce
/// `isolate channel connection was closed` y deja la UI a medias.
class AppDatabaseManager {
  AppDatabaseManager._();

  static final AppDatabaseManager instance = AppDatabaseManager._();

  AppDatabase? _db;
  Future<AppDatabase>? _opening;
  int _leases = 0;
  int _activeOperations = 0;
  bool _closed = false;

  bool get isClosed => _closed;
  int get leases => _leases;
  int get activeOperations => _activeOperations;

  /// Devuelve la base compartida, abriéndola (y migrándola) una sola vez.
  Future<AppDatabase> acquire() async {
    if (_closed) {
      throw StateError('AppDatabaseManager ya fue cerrado');
    }
    _leases++;
    try {
      final existing = _db;
      if (existing != null) return existing;
      final opening = _opening;
      if (opening != null) return await opening;
      final future = _doOpen();
      _opening = future;
      try {
        return await future;
      } finally {
        if (identical(_opening, future)) _opening = null;
      }
    } catch (_) {
      if (_leases > 0) _leases--;
      rethrow;
    }
  }

  Future<AppDatabase> _doOpen() async {
    final db = AppDatabase();
    try {
      // Fuerza la apertura/migración dentro del isolate antes de compartirla.
      await db.customSelect('SELECT 1').get();
      _db = db;
      return db;
    } catch (_) {
      try {
        await db.close();
      } catch (_) {
        // La apertura ya falló; el cierre es best-effort.
      }
      rethrow;
    }
  }

  /// Marca que una pantalla dejó de usar la base. No cierra la conexión.
  void release() {
    if (_leases > 0) _leases--;
  }

  /// Envuelve una operación asíncrona para que [close] espere a que termine.
  Future<T> guard<T>(Future<T> Function() operation) async {
    if (_closed) {
      throw StateError('AppDatabaseManager ya fue cerrado');
    }
    _activeOperations++;
    try {
      return await operation();
    } finally {
      if (_activeOperations > 0) _activeOperations--;
    }
  }

  /// Cierre controlado (al salir de la app).
  ///
  /// Espera hasta [timeout] a que no queden operaciones en vuelo y luego
  /// cierra el isolate. Si alguna operación sigue viva, el proceso de salida
  /// termina igualmente; no se bloquea la app indefinidamente.
  Future<void> close({Duration timeout = const Duration(seconds: 3)}) async {
    if (_closed) return;
    _closed = true;

    final opening = _opening;
    if (opening != null) {
      try {
        await opening;
      } catch (_) {
        // Si la apertura falló no hay nada que cerrar.
      }
    }

    final deadline = DateTime.now().add(timeout);
    while (_activeOperations > 0 && DateTime.now().isBefore(deadline)) {
      await Future<void>.delayed(const Duration(milliseconds: 50));
    }

    final db = _db;
    _db = null;
    if (db == null) return;
    try {
      await db.close();
    } catch (e) {
      debugPrint('[DB] cierre controlado ignoró error: $e');
    }
  }

  /// Solo tests: cierra y deja el manager reutilizable en el siguiente test.
  @visibleForTesting
  Future<void> resetForTesting() async {
    await close();
    _closed = false;
    _leases = 0;
    _activeOperations = 0;
    _opening = null;
    _db = null;
  }
}
