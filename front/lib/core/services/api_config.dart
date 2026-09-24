// Se definen al compilar: --dart-define=API_BASE_URL=... --dart-define=NOTES_BASE_URL=...
// Un valor que empieza con "/" es relativo al origen de la página (ej. /api-auth).
const _authBase =
    String.fromEnvironment('API_BASE_URL', defaultValue: 'http://localhost:8085');
const _notesBase =
    String.fromEnvironment('NOTES_BASE_URL', defaultValue: 'http://localhost:8082');

String _resolve(String v) => v.startsWith('/') ? '${Uri.base.origin}$v' : v;

String get authApiBaseUrl => _resolve(_authBase);
String get notesApiBaseUrl => _resolve(_notesBase);
