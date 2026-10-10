import 'package:flutter/foundation.dart' show kIsWeb;

// Web y aplicaciones instaladas usan los mismos servicios y PostgreSQL de Pillán.
// Desarrollo local sigue disponible con --dart-define (front/.env.local.example).
const _pillanOrigin = 'https://ti3-brojas.dev.censei.cl';
const _authBase = String.fromEnvironment(
  'API_BASE_URL',
  defaultValue: '$_pillanOrigin/api-auth',
);
const _notesBase = String.fromEnvironment(
  'NOTES_BASE_URL',
  defaultValue: '$_pillanOrigin/api-notes',
);
const _socialBase = String.fromEnvironment(
  'SOCIAL_BASE_URL',
  defaultValue: '$_pillanOrigin/api-social',
);

String _resolve(String v) {
  if (!v.startsWith('/')) return v;
  if (kIsWeb) return '${Uri.base.origin}$v';
  // Nativo no tiene origen web. Los paths del build web apuntan también a Pillán.
  return '$_pillanOrigin$v';
}

String get authApiBaseUrl => _resolve(_authBase);
String get notesApiBaseUrl => _resolve(_notesBase);
String get socialApiBaseUrl => _resolve(_socialBase);
