import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/auth/google_drive_service.dart';

void main() {
  group('GoogleDriveService.extractOAuthCode', () {
    test('código crudo se conserva', () {
      expect(GoogleDriveService.extractOAuthCode('4/0AabcDEF-123'), '4/0AabcDEF-123');
    });

    test('URL completa con code sin codificar', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?code=4/0Aabc&scope=email',
        ),
        '4/0Aabc',
      );
    });

    test('URL completa con code URL-encoded se decodifica', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?code=4%2F0Axyz-_.&scope=email',
        ),
        '4/0Axyz-_.',
      );
    });

    test('query code=...', () {
      expect(
        GoogleDriveService.extractOAuthCode('code=ABC123&scope=email'),
        'ABC123',
      );
    });

    test('retorno con error= no produce código', () {
      expect(
        GoogleDriveService.extractOAuthCode(
          'http://localhost:8081/auth/google/callback?error=access_denied',
        ),
        '',
      );
    });

    test('vacío devuelve vacío', () {
      expect(GoogleDriveService.extractOAuthCode('   '), '');
    });
  });
}
