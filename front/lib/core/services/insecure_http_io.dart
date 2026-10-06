import 'dart:io';
import 'package:crypto/crypto.dart';

// Certificado público observado en Pillán. Una rotación exige verificar y
// actualizar la huella; nunca se aceptan certificados arbitrarios por hostname.
const pillanCertificateSha256 = String.fromEnvironment(
  'PILLAN_CERT_SHA256',
  defaultValue:
      '19c7ea107b9d3063cf86f9a204bd75d9bc220cc08f4bc10942a4c854c06f7665,'
      'aed37feef51290d0fc0b4dded16770b3deac9970feb7128b5ae54b65f5bbcdf6,'
      'e85a783d4d7bcbe0bfebf0a1b27ac6d4f400ee4fd8c0acf69f8d914e243b458a',
);

bool isTrustedPillanCertificate(X509Certificate cert, String host, int port) {
  final now = DateTime.now();
  return host == 'ti3-brojas.dev.censei.cl' &&
      port == 443 &&
      !now.isBefore(cert.startValidity) &&
      now.isBefore(cert.endValidity) &&
      pillanCertificateSha256
          .toLowerCase()
          .split(',')
          .map((pin) => pin.trim())
          .contains(sha256.convert(cert.der).toString());
}

class _PillanCertificateOverrides extends HttpOverrides {
  @override
  HttpClient createHttpClient(SecurityContext? context) {
    final client = super.createHttpClient(context);
    client.badCertificateCallback = isTrustedPillanCertificate;
    return client;
  }
}

void configurePillanCertificateTrust() {
  HttpOverrides.global = _PillanCertificateOverrides();
}
