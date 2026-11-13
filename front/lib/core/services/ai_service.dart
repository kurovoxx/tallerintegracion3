import 'ai_service_web.dart' if (dart.library.io) 'ai_service_io.dart';

import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;

import 'api_config.dart';
import 'session_manager.dart';

// Cliente para el microservicio IA (desplegado en cluster externo).
// Requiere autenticación JWT (Authorization: Bearer <token>).
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
    final token = SessionManager.token;
    final request = http.MultipartRequest(
      'POST',
      Uri.parse('$baseUrl/ai/pdf-to-markdown'),
    );
    request.files.add(
      http.MultipartFile.fromBytes('file', pdfBytes, filename: filename),
    );
    // Agregar header de autenticación JWT si hay sesión.
    if (token != null && token.isNotEmpty) {
      request.headers['Authorization'] = 'Bearer $token';
    }

    try {
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
    } on AiApiException {
      rethrow;
    } catch (e, st) {
      print('[IA] Error de red/SSL: $e\n$st');
      final msg = e.toString();
      if (msg.contains('CERTIFICATE_VERIFY_FAILED') ||
          msg.contains('HandshakeException') ||
          msg.contains('failed to fetch') ||
          msg.contains('SSL')) {
        throw AiApiException(
          'No se pudo conectar con el servicio de IA. El certificado SSL del servidor no es válido para el navegador. Contacta al administrador para configurar un certificado SSL válido en ia-hlepio.dev.censei.cl.',
          statusCode: 0,
          code: 'ssl_certificate_invalid',
        );
      }
      if (msg.contains('Connection closed before full header') ||
          msg.contains('Connection closed') ||
          msg.contains('ClientException')) {
        throw AiApiException(
          'La conexión con el servidor de IA se cerró antes de recibir la respuesta. Esto suele deberse a que el archivo excede el tamaño permitido en el servidor (intenta con un PDF más pequeño) o a que el servidor tardó demasiado en responder. Contacta al administrador de ia-hlepio.dev.censei.cl.',
          statusCode: 0,
          code: 'connection_closed',
        );
      }
      throw AiApiException(
        'Error de red al conectar con el servicio de IA: $e',
        statusCode: 0,
        code: 'network_error',
      );
    }
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
