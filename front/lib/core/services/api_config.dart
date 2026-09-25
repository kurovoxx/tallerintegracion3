import 'package:flutter/foundation.dart' show kIsWeb;

// Local por defecto (http://localhost:8085) para `flutter run -d linux` y Docker local sin .env.
// Para dominio ngrok/tunnel, build con --dart-define o .env.prod (ver front/.env.prod.example)
// Para web con nginx, también puedes usar relativo '/api-auth' -> _resolve lo convierte a origin.
const _authBase =
    String.fromEnvironment('API_BASE_URL', defaultValue: 'http://localhost:8085');
const _notesBase =
    String.fromEnvironment('NOTES_BASE_URL', defaultValue: 'http://localhost:8082');
const _socialBase =
    String.fromEnvironment('SOCIAL_BASE_URL', defaultValue: 'http://localhost:8083');

String _resolve(String v) {
  if (!v.startsWith('/')) return v;
  if (kIsWeb) return '${Uri.base.origin}$v';
  // Nativo (linux/windows) no tiene origin http, mapea relativo a localhost
  if (v == '/api-auth') return 'http://localhost:8085';
  if (v == '/api-notes') return 'http://localhost:8082';
  if (v == '/api-social') return 'http://localhost:8083';
  return v;
}

String get authApiBaseUrl => _resolve(_authBase);
String get notesApiBaseUrl => _resolve(_notesBase);
String get socialApiBaseUrl => _resolve(_socialBase);
