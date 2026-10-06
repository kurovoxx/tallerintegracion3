// Confianza explícita en el certificado de desarrollo de Pillán mediante SHA-256.
// Los demás certificados siguen la validación TLS normal. Web la delega al navegador.
export 'insecure_http_stub.dart' if (dart.library.io) 'insecure_http_io.dart';
