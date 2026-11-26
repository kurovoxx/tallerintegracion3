import 'package:flutter/foundation.dart'
    show TargetPlatform, defaultTargetPlatform, kIsWeb;

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

// Microservicio IA (desplegado en cluster externo, no en Pillán).
const _aiBase = String.fromEnvironment(
  'AI_BASE_URL',
  defaultValue: 'https://ia-hlepio.dev.censei.cl',
);

bool get _isAndroidNative =>
    !kIsWeb && defaultTargetPlatform == TargetPlatform.android;

String resolveApiBaseUrl(
  String v, {
  bool isWeb = kIsWeb,
  String? webOrigin,
  bool? isAndroid,
}) {
  if (v.startsWith('/')) {
    if (isWeb) return '${webOrigin ?? Uri.base.origin}$v';
    // Nativo no tiene origen web. Los paths del build web apuntan también a Pillán.
    return '$_pillanOrigin$v';
  }
  // En Android, si se configuró explícitamente un backend local con localhost
  // o 127.0.0.1 mediante --dart-define, se traduce a 10.0.2.2 para el emulador.
  if (!isWeb && (isAndroid ?? _isAndroidNative)) {
    return v.replaceFirst(
      RegExp(r'^http://(localhost|127\.0\.0\.1)(?=[:/]|$)'),
      'http://10.0.2.2',
    );
  }
  return v;
}

String get authApiBaseUrl => resolveApiBaseUrl(_authBase);
String get notesApiBaseUrl => resolveApiBaseUrl(_notesBase);
String get socialApiBaseUrl => resolveApiBaseUrl(_socialBase);
String get aiApiBaseUrl => resolveApiBaseUrl(_aiBase);
