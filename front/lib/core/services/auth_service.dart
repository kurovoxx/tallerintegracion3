import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

import 'api_config.dart';

class AuthService {
  AuthService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  // Backend Gin - auth-service
  // Docker: auth en 8085:8080 (host 8081 reservado para callback OAuth local de Drive) -> http://localhost:8085
  // Host directo (go run): PORT=8082 -> http://localhost:8082
  // Rutas reales: POST /auth/register y POST /auth/login (sin /api)
  // Ver back/auth/cmd/server/main.go:73 y agentApiContract.md:22 y docker-compose.yml:28
  static String get baseUrl => authApiBaseUrl;
  static const String loginPath = '/auth/login';
  static const String registerPath = '/auth/register';

  Future<LoginResult> login({
    required String email,
    required String password,
  }) async {
    final payload = <String, String>{
      'email': email.trim(),
      'password': password,
    };

    debugPrint('Payload login preparado para email: ${email.trim()}');

    try {
      final response = await _client
          .post(
            Uri.parse('$baseUrl$loginPath'),
            headers: const {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10));

      final Map<String, dynamic> responseBody;
      if (response.body.isEmpty) {
        responseBody = <String, dynamic>{};
      } else {
        responseBody = jsonDecode(response.body) as Map<String, dynamic>;
      }

      if (response.statusCode >= 200 && response.statusCode < 300) {
        return LoginResult.success(responseBody);
      }

      return LoginResult.failure(_extractErrorMessage(responseBody, response.statusCode));
    } on FormatException {
      return LoginResult.failure('La API respondió con un JSON inválido.');
    } on Exception {
      return LoginResult.failure(
        'No se pudo conectar con el servidor. Revisa la URL ($baseUrl), el puerto y que Gin esté ejecutándose.',
      );
    } catch (e) {
      // Captura también Error (ej. file:///api-auth en nativo con relativo mal configurado)
      return LoginResult.failure(
        'No se pudo conectar con el servidor. Revisa la URL ($baseUrl), el puerto y que Gin esté ejecutándose. ($e)',
      );
    }
  }

  Future<RegisterResult> register({
    required String email,
    required String password,
    required String displayName,
    String role = 'student',
    String? institution,
    String? photoUrl,
    String? phone,
    String? description,
    String? visibility,
  }) async {
    final payload = <String, dynamic>{
      'email': email.trim().toLowerCase(),
      'password': password,
      'role': role,
      'display_name': displayName.trim(),
      if (institution != null && institution.trim().isNotEmpty) 'institution': institution.trim(),
      if (photoUrl != null && photoUrl.trim().isNotEmpty) 'photo_url': photoUrl.trim(),
      if (phone != null && phone.trim().isNotEmpty) 'phone': phone.trim(),
      if (description != null && description.trim().isNotEmpty) 'description': description.trim(),
      if (visibility != null) 'visibility': visibility,
    };

    debugPrint('Payload register: email=${email.trim()}, display_name=${displayName.trim()}, role=$role');

    try {
      final response = await _client
          .post(
            Uri.parse('$baseUrl$registerPath'),
            headers: const {
              'Content-Type': 'application/json',
              'Accept': 'application/json',
            },
            body: jsonEncode(payload),
          )
          .timeout(const Duration(seconds: 10));

      final Map<String, dynamic> responseBody;
      if (response.body.isEmpty) {
        responseBody = <String, dynamic>{};
      } else {
        responseBody = jsonDecode(response.body) as Map<String, dynamic>;
      }

      if (response.statusCode == 201) {
        return RegisterResult.success(responseBody);
      }

      return RegisterResult.failure(_extractErrorMessage(responseBody, response.statusCode));
    } on FormatException {
      return RegisterResult.failure('La API respondió con un JSON inválido.');
    } on Exception {
      return RegisterResult.failure(
        'No se pudo conectar con el servidor. Revisa la URL ($baseUrl), el puerto y que Gin esté ejecutándose.',
      );
    } catch (e) {
      return RegisterResult.failure(
        'No se pudo conectar con el servidor. Revisa la URL ($baseUrl), el puerto y que Gin esté ejecutándose. ($e)',
      );
    }
  }

  String _extractErrorMessage(Map<String, dynamic> body, int statusCode) {
    // Contrato: { "error": { "code": "string", "message": "string" } }
    if (body.containsKey('error')) {
      final err = body['error'];
      if (err is Map) {
        final msg = err['message'];
        final code = err['code'];
        if (msg is String && msg.isNotEmpty) {
          // Mapear códigos a mensajes más amigables si hace falta
          switch (code) {
            case 'email_taken':
              return 'Ese correo ya está registrado.';
            case 'invalid_email':
              return 'Formato de correo inválido.';
            case 'weak_password':
              return 'Contraseña débil: mínimo 8 caracteres, al menos una letra y un dígito, sin espacios.';
            case 'invalid_role':
              return 'Rol inválido (debe ser student o teacher).';
            case 'invalid_display_name':
              return 'Nombre inválido (1-255 caracteres).';
            case 'invalid_visibility':
              return 'Visibilidad inválida.';
            case 'invalid_credentials':
              return 'Credenciales inválidas.';
            default:
              return msg;
          }
        }
        if (code is String && code.isNotEmpty) return code;
      }
      if (err is String && err.isNotEmpty) return err;
    }
    // Fallback genérico por status
    if (statusCode == 409) return 'Conflicto: recurso ya existe.';
    if (statusCode == 401) return 'No autorizado.';
    if (statusCode == 400) return 'Datos inválidos.';
    return 'No fue posible completar la solicitud. Código HTTP: $statusCode';
  }

  void dispose() {
    _client.close();
  }
}

/// Resultado login
class LoginResult {
  const LoginResult._({
    required this.success,
    this.data,
    this.message,
  });

  final bool success;
  final Map<String, dynamic>? data;
  final String? message;

  factory LoginResult.success(Map<String, dynamic> data) {
    return LoginResult._(success: true, data: data);
  }

  factory LoginResult.failure(String message) {
    return LoginResult._(success: false, message: message);
  }
}

/// Resultado registro
class RegisterResult {
  const RegisterResult._({
    required this.success,
    this.data,
    this.message,
  });

  final bool success;
  final Map<String, dynamic>? data;
  final String? message;

  factory RegisterResult.success(Map<String, dynamic> data) {
    return RegisterResult._(success: true, data: data);
  }

  factory RegisterResult.failure(String message) {
    return RegisterResult._(success: false, message: message);
  }
}
