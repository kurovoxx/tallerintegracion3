import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
class AuthService {
  AuthService({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  // Para Flutter Windows y backend Gin en ESTE mismo computador.
  // Cuando Martín confirme URL/ruta, modifica solo estas constantes.
  static const String baseUrl = 'http://localhost:8080';
  static const String loginPath = '/api/auth/login';

  Future<LoginResult> login({
    required String email,
    required String password,
  }) async {
    // ===== PAYLOAD DE LA TAREA 1_2_13 =====
    //
    // jsonEncode lo transforma exactamente en:
    // {
    //   "email": "alguien@alu.uct.cl",
    //   "password": "claveDelUsuario"
    // }
    final payload = <String, String>{
      'email': email.trim(),
      'password': password,
    };

    // Esto permite demostrar y verificar qué JSON se está preparando.
    // OJO: cuando el proyecto sea real, NO imprimas la contraseña.
    debugPrint(
    'Payload login preparado para email: ${email.trim()}',
    );

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

      return LoginResult.failure(
        responseBody['error']?.toString() ??
            'No fue posible iniciar sesión. Código HTTP: ${response.statusCode}',
      );
    } on FormatException {
      return LoginResult.failure(
        'La API respondió con un JSON inválido.',
      );
    } on Exception {
      return LoginResult.failure(
        'No se pudo conectar con el servidor. Revisa la URL, el puerto y que Gin esté ejecutándose.',
      );
    }
  }

  void dispose() {
    _client.close();
  }
}

/// Resultado que recibe LoginScreen sin tener que manejar directamente
/// códigos HTTP ni parseo de JSON.
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