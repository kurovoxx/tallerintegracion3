// Estático: dominio fijo de ngrok que apunta al front en pillan (nginx reenvía /api-*).
// Si cambia el dominio de ngrok, actualizar estos 3 valores (también Dockerfile y docker-compose.yml) y rebuild.
const _authBase = String.fromEnvironment('API_BASE_URL',
    defaultValue: 'https://dotted-unaudited-liking.ngrok-free.dev/api-auth');
const _notesBase = String.fromEnvironment('NOTES_BASE_URL',
    defaultValue: 'https://dotted-unaudited-liking.ngrok-free.dev/api-notes');
const _socialBase = String.fromEnvironment('SOCIAL_BASE_URL',
    defaultValue: 'https://dotted-unaudited-liking.ngrok-free.dev/api-social');

String _resolve(String v) => v.startsWith('/') ? '${Uri.base.origin}$v' : v;

String get authApiBaseUrl => _resolve(_authBase);
String get notesApiBaseUrl => _resolve(_notesBase);
String get socialApiBaseUrl => _resolve(_socialBase);
