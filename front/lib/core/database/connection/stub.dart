import 'package:drift/drift.dart';

/// Stub para plataformas no soportadas. Nunca se usa en runtime
/// si los condicionales if (dart.library.io/html) están bien configurados.
QueryExecutor openConnection() {
  throw UnsupportedError('No hay implementación de DB para esta plataforma');
}
