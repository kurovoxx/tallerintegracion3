// Estático para test login vía front contra túnel público actual.
// Si el pod tunnel reinicia y cambia la URL trycloudflare, actualizar estos 3 valores y rebuild.
const _authBase = String.fromEnvironment('API_BASE_URL',
    defaultValue: 'https://gcc-too-virgin-rhode.trycloudflare.com/api-auth');
const _notesBase = String.fromEnvironment('NOTES_BASE_URL',
    defaultValue: 'https://gcc-too-virgin-rhode.trycloudflare.com/api-notes');
const _socialBase = String.fromEnvironment('SOCIAL_BASE_URL',
    defaultValue: 'https://gcc-too-virgin-rhode.trycloudflare.com/api-social');

String _resolve(String v) => v.startsWith('/') ? '${Uri.base.origin}$v' : v;

String get authApiBaseUrl => _resolve(_authBase);
String get notesApiBaseUrl => _resolve(_notesBase);
String get socialApiBaseUrl => _resolve(_socialBase);
