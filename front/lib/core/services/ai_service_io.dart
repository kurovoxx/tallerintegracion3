import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:http/io_client.dart';

/// Crea un HttpClient que acepta certificados autofirmados
/// (solo para móvil/desktop; en web no podemos saltarnos la verificación SSL).
http.Client createIoClient() {
  final ioClient = HttpClient()
    ..badCertificateCallback = (cert, host, port) => true;
  return IOClient(ioClient);
}
