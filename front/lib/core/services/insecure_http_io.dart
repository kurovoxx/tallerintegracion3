import 'dart:io';
import 'package:crypto/crypto.dart';

// Certificado público observado en Pillán. Una rotación exige verificar y
// actualizar la huella; nunca se aceptan certificados arbitrarios por hostname.
const pillanCertificateSha256 = String.fromEnvironment(
  'PILLAN_CERT_SHA256',
  defaultValue:
      '38d5ec7aa9fd2e2323ef73055e40c888927191a813320e04e02f607d417d395d,'
      'bbc1e730ad1937be888500b3705abcb7560990ba9a590041ff5359c4f69480aa,'
      'd68cbdf6646dc6e49c17d01d0282a6d214c6acd9d2845278ef89701a0200ec7e',
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
