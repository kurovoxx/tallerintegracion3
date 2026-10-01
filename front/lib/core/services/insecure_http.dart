// Bypass TLS solo para desarrollo desktop contra el Ingress con cert self-signed.
// Se activa con --dart-define=ALLOW_INSECURE=true (ver scripts/run-front.sh opción 5).
// Solo permite el host de pillan, no es un bypass global.
// En web usa el stub (no-op): el navegador gestiona el TLS.
export 'insecure_http_stub.dart' if (dart.library.io) 'insecure_http_io.dart';
