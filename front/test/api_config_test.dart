import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/services/api_config.dart';

void main() {
  test('native defaults and relative web build paths both use Pillan', () {
    expect(authApiBaseUrl, 'https://ti3-brojas.dev.censei.cl/api-auth');
    expect(notesApiBaseUrl, 'https://ti3-brojas.dev.censei.cl/api-notes');
    expect(socialApiBaseUrl, 'https://ti3-brojas.dev.censei.cl/api-social');
  });
}
