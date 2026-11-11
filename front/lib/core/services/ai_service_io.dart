import 'dart:io';

import 'package:http/http.dart' as http;
import 'package:http/io_client.dart';

import 'insecure_http_io.dart';

/// Cliente para móvil/desktop. El Ingress sirve el "Fake Certificate":
/// solo se acepta si coincide con las huellas fijadas en
/// [isTrustedPillanCertificate] (host + puerto + SHA-256), nunca cualquiera.
http.Client createIoClient() {
  final ioClient = HttpClient()
    ..badCertificateCallback = isTrustedPillanCertificate;
  return IOClient(ioClient);
}
