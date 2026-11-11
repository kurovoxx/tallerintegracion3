import 'package:http/http.dart' as http;

/// En web no podemos saltarnos la verificación SSL desde Dart:
/// es una limitación del navegador. El usuario debe aceptar el
/// certificado manualmente en el navegador, o el servidor debe
/// tener un certificado válido.
http.Client createIoClient() => http.Client();
