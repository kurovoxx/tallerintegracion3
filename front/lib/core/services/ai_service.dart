import 'ai_service_web.dart' if (dart.library.io) 'ai_service_io.dart';

import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'api_config.dart';

// Cliente para el microservicio IA (desplegado en cluster externo).
// No requiere autenticación (CORS activado en el Ingress).
// Contrato:
//   POST /ai/pdf-to-markdown (multipart/form-data, campo "file")
//   -> 200 {markdown, filename, size_bytes, provider}
class AiService {
  AiService({http.Client? client}) : _client = client ?? createIoClient();

  final http.Client _client;

  static String get baseUrl => aiApiBaseUrl;

  /// Sube un PDF y devuelve el Markdown convertido por Gemini.
  /// Lanza [AiApiException] si el backend responde con error.
  Future<AiConversionResult> convertPdfToMarkdown(Uint8List pdfBytes, String filename) async {
    final request = http.MultipartRequest(
      'POST',
      Uri.parse('$baseUrl/ai/pdf-to-markdown'),
    );
    request.files.add(
      http.MultipartFile.fromBytes('file', pdfBytes, filename: filename),
    );

    final streamedResponse = await _client.send(request).timeout(
      const Duration(seconds: 60),
    );
    final res = await http.Response.fromStream(streamedResponse);

    if (res.statusCode != 200) throw _toError(res);

    final body = jsonDecode(utf8.decode(res.bodyBytes)) as Map<String, dynamic>;
    return AiConversionResult(
      markdown: body['markdown'] as String? ?? '',
      filename: body['filename'] as String? ?? filename,
      sizeBytes: body['size_bytes'] as int? ?? pdfBytes.length,
      provider: body['provider'] as String? ?? 'gemini',
    );
  }

  AiApiException _toError(http.Response res) {
    String message = 'Error HTTP ${res.statusCode}';
    String? code;
    try {
      if (res.body.isNotEmpty) {
        final body = jsonDecode(utf8.decode(res.bodyBytes));
        if (body is Map) {
          final err = body['error'];
          if (err is Map) {
            code = err['code']?.toString();
            message = err['message']?.toString() ?? err.toString();
          } else if (err is String) {
            message = err;
          } else if (body['message'] is String) {
            message = body['message'] as String;
          }
        }
      }
    } catch (_) {}
    return AiApiException(message, statusCode: res.statusCode, code: code);
  }

  void dispose() {
    _client.close();
  }
}

class AiConversionResult {
  AiConversionResult({
    required this.markdown,
    required this.filename,
    required this.sizeBytes,
    required this.provider,
  });

  final String markdown;
  final String filename;
  final int sizeBytes;
  final String provider;
}

class AiApiException implements Exception {
  AiApiException(this.message, {this.statusCode, this.code});

  final String message;
  final int? statusCode;
  final String? code;

  @override
  String toString() => 'AiApiException: $message (status: $statusCode, code: $code)';
}
