import 'dart:io';

// Solo-dev: el Ingress de pillan sirve Fake Certificate (sin tls: en k8s/ingress.yaml).
// Dart nativo valida el cert y falla con HandshakeException, mientras la web
// con /api-* mismo-origen lo tolera. Este override acepta SOLO el host de pillan
// y SOLO si se compila con --dart-define=ALLOW_INSECURE=true.
const _allowInsecure = bool.fromEnvironment(
  'ALLOW_INSECURE',
  defaultValue: false,
);

const _allowedHosts = <String>{'ti3-brojas.dev.censei.cl'};

class _CenseiInsecureOverrides extends HttpOverrides {
  @override
  HttpClient createHttpClient(SecurityContext? context) {
    final client = super.createHttpClient(context);
    client.badCertificateCallback =
        (X509Certificate cert, String host, int port) {
          if (_allowedHosts.contains(host)) return true;
          return false;
        };
    return client;
  }
}

void maybeAllowInsecureCerts() {
  if (!_allowInsecure) return;
  HttpOverrides.global = _CenseiInsecureOverrides();
}
