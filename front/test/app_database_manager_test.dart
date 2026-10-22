import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/database/app_database_manager.dart';
import 'package:taller_integracion_front/core/database/local_notes_repository.dart';
import 'package:taller_integracion_front/core/database/app_database.dart';

// 3_2_18 / #12: el gestor de SQLite no cierra conexiones con operaciones vivas.
void main() {
  final manager = AppDatabaseManager.instance;

  tearDown(() async {
    await manager.resetForTesting();
  });

  test('acquire comparte UNA instancia y release no cierra la base', () async {
    final a = await manager.acquire();
    final b = await manager.acquire();
    expect(identical(a, b), isTrue);
    expect(manager.leases, 2);

    // Una pantalla sale (dispose): la otra sigue usando la misma conexión.
    manager.release();
    expect(manager.leases, 1);
    expect(manager.isClosed, isFalse);
    final rows = await LocalNotesRepository(b).getAllNotes();
    expect(rows, isEmpty);
  });

  test('aperturas concurrentes no crean dos conexiones', () async {
    final results = await Future.wait([
      manager.acquire(),
      manager.acquire(),
      manager.acquire(),
    ]);
    expect(identical(results[0], results[1]), isTrue);
    expect(identical(results[1], results[2]), isTrue);
  });

  test('close espera a que termine la operación en vuelo', () async {
    final db = await manager.acquire();
    final repo = LocalNotesRepository(db);
    final gate = Completer<void>();
    var wrote = false;

    final op = manager.guard(() async {
      await gate.future;
      await repo.upsertNote(
        LocalNote(
          id: '1',
          title: 'en vuelo',
          content: 'x',
          visibility: 'private',
          updatedAt: DateTime.now(),
        ),
      );
      wrote = true;
    });
    expect(manager.activeOperations, 1);

    final closing = manager.close(timeout: const Duration(seconds: 5));
    // El cierre no completa mientras la operación sigue viva.
    await Future<void>.delayed(const Duration(milliseconds: 150));
    expect(wrote, isFalse);

    gate.complete();
    await op;
    await closing;
    expect(wrote, isTrue, reason: 'la escritura terminó antes del cierre');
    expect(manager.isClosed, isTrue);
    expect(manager.activeOperations, 0);
  });

  test('tras cerrar, acquire y guard fallan de forma explícita', () async {
    await manager.acquire();
    await manager.close();
    expect(manager.acquire, throwsStateError);
    expect(() => manager.guard(() async => 1), throwsStateError);
  });

  test('una operación que falla no deja el contador colgado', () async {
    await manager.acquire();
    await expectLater(
      manager.guard<void>(() async => throw StateError('boom')),
      throwsStateError,
    );
    expect(manager.activeOperations, 0);
  });
}
