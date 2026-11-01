import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/services/api_config.dart';

void main() {
  test('bases respetan la configuración compilada', () {
    const origin = 'https://ti3-brojas.dev.censei.cl';
    const auth = String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: '$origin/api-auth',
    );
    const notes = String.fromEnvironment(
      'NOTES_BASE_URL',
      defaultValue: '$origin/api-notes',
    );
    const social = String.fromEnvironment(
      'SOCIAL_BASE_URL',
      defaultValue: '$origin/api-social',
    );
    expect(authApiBaseUrl, auth.startsWith('/') ? '$origin$auth' : auth);
    expect(notesApiBaseUrl, notes.startsWith('/') ? '$origin$notes' : notes);
    expect(
      socialApiBaseUrl,
      social.startsWith('/') ? '$origin$social' : social,
    );
  });
  test('rutas relativas usan Pillan nativo y el origen actual en web', () {
    expect(
      resolveApiBaseUrl('/api-notes', isWeb: false),
      'https://ti3-brojas.dev.censei.cl/api-notes',
    );
    expect(
      resolveApiBaseUrl(
        '/api-notes',
        isWeb: true,
        webOrigin: 'https://test.invalid',
      ),
      'https://test.invalid/api-notes',
    );
    expect(
      resolveApiBaseUrl('http://localhost:18082', isWeb: true),
      'http://localhost:18082',
    );
  });
}
