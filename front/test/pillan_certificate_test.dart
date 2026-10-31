import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/services/insecure_http_io.dart';

class TestCertificate extends Fake implements X509Certificate {
  TestCertificate(this.der, {this.expired = false});
  @override
  final Uint8List der;
  final bool expired;
  @override
  DateTime get startValidity =>
      DateTime.now().subtract(const Duration(days: 2));
  @override
  DateTime get endValidity =>
      DateTime.now().add(Duration(days: expired ? -1 : 1));
}

void main() {
  // Fixture histórico retirado del pinning productivo en 43b654b.
  const fixturePin =
      '19c7ea107b9d3063cf86f9a204bd75d9bc220cc08f4bc10942a4c854c06f7665';
  final bytes = File('test/fixtures/pillan_ingress.der').readAsBytesSync();
  test('configuración productiva no acepta un certificado retirado', () {
    final pins = pillanCertificateSha256
        .toLowerCase()
        .split(',')
        .map((s) => s.trim());
    expect(
      isTrustedPillanCertificate(
        TestCertificate(bytes),
        'ti3-brojas.dev.censei.cl',
        443,
      ),
      pins.contains(fixturePin),
    );
  });
  test(
    'accepts only the pinned Pillan certificate on the intended host/port',
    () {
      expect(
        isTrustedPillanCertificate(
          TestCertificate(bytes),
          'ti3-brojas.dev.censei.cl',
          443,
          fingerprints: fixturePin,
        ),
        isTrue,
      );
      expect(
        isTrustedPillanCertificate(
          TestCertificate(bytes),
          'other.example.com',
          443,
          fingerprints: fixturePin,
        ),
        isFalse,
      );
      expect(
        isTrustedPillanCertificate(
          TestCertificate(bytes),
          'ti3-brojas.dev.censei.cl',
          8443,
          fingerprints: fixturePin,
        ),
        isFalse,
      );
      expect(
        isTrustedPillanCertificate(
          TestCertificate(bytes, expired: true),
          'ti3-brojas.dev.censei.cl',
          443,
          fingerprints: fixturePin,
        ),
        isFalse,
      );
      expect(
        isTrustedPillanCertificate(
          TestCertificate(Uint8List.fromList([1, 2, 3])),
          'ti3-brojas.dev.censei.cl',
          443,
          fingerprints: fixturePin,
        ),
        isFalse,
      );
    },
  );
}
