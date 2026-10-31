import 'dart:convert';
import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:google_sign_in/google_sign_in.dart';
import 'package:http/http.dart' as http;

import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';
import 'google_drive_service.dart' show GoogleDriveService;

/// Resultado de POST /auth/google-calendar/connect. Mensajes listos para UI.
class CalendarConnectResult {
  const CalendarConnectResult._(this.ok, this.message);
  const CalendarConnectResult.ok() : this._(true, '');
  const CalendarConnectResult.error(String message) : this._(false, message);

  final bool ok;
  final String message;
}

/// Estado de GET /auth/google-calendar/status. Sin tokens ni secretos.
class CalendarStatus {
  const CalendarStatus({required this.connected, this.externalEmail});

  final bool connected;
  final String? externalEmail;
}

/// Servicio para Google Calendar real. Reutiliza la infraestructura que ya
/// funciona para Drive (config remota, loopback desktop con puerto efímero,
/// google_sign_in móvil) parametrizando únicamente el scope real de
/// Calendar. NO rompe Drive: su servicio conserva sus defaults.
class GoogleCalendarService {
  GoogleCalendarService({http.Client? client}) : _client = client;

  final http.Client? _client;

  static Future<void> ensureConfigured({required String backendBaseUrl}) =>
      GoogleDriveService.ensureConfigured(backendBaseUrl: backendBaseUrl);

  static bool get isConfigured => GoogleDriveService.isConfigured;

  static const calendarScopes =
      'https://www.googleapis.com/auth/calendar.events%20openid%20email%20profile';

  final GoogleDriveService _drive = GoogleDriveService();

  final GoogleSignIn _googleSignIn = GoogleSignIn(
    scopes: [
      'https://www.googleapis.com/auth/calendar.events',
      'openid',
      'email',
      'profile',
    ],
  );

  /// Código OAuth con scope de Calendar + redirect real usado (puerto
  /// efímero en desktop). null = usuario canceló. En web retorna null
  /// (usar flujo manual pegar-código).
  Future<({String code, String redirectUri})?> getCalendarAuthCode({
    String? loginHint,
  }) async {
    if (!isConfigured) {
      throw StateError('GOOGLE_CLIENT_ID no configurado.');
    }
    if (kIsWeb) return null;
    if (!kIsWeb && (Platform.isWindows || Platform.isLinux)) {
      final auth = await _drive.getDesktopAuthCode(
        loginHint: loginHint,
        scopes: calendarScopes,
      );
      if (auth == null) return null;
      return (code: auth.code, redirectUri: auth.redirectUri);
    }
    final account = await _googleSignIn.signIn();
    if (account == null) return null;
    final code = account.serverAuthCode;
    if (code == null || code.isEmpty) return null;
    return (code: code, redirectUri: '');
  }

  /// Envía oauth_code (+ redirect real en desktop) al backend.
  Future<CalendarConnectResult> connectCalendar({
    required String backendBaseUrl,
    required String oauthCode,
    String? redirectUri,
    int? expectedSessionRevision,
  }) async {
    final revision = expectedSessionRevision ?? SessionManager.revision.value;
    final base = backendBaseUrl.trim().replaceFirst(RegExp(r'/+$'), '');
    if (base.isEmpty || oauthCode.trim().isEmpty) {
      return const CalendarConnectResult.error(
        'No se pudo completar la conexión. Inténtalo nuevamente.',
      );
    }
    final payload = <String, String>{'oauth_code': oauthCode.trim()};
    final redirect = redirectUri?.trim() ?? '';
    if (redirect.isNotEmpty) payload['redirect_uri'] = redirect;
    http.Response resp;
    try {
      resp = await AuthedHttp.run(() {
        if (revision != SessionManager.revision.value) {
          throw StateError('La sesión cambió.');
        }
        final token = SessionManager.token;
        if (token == null || token.isEmpty) {
          throw StateError('No hay una sesión activa.');
        }
        final url = Uri.parse('$base/auth/google-calendar/connect');
        final headers = {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer $token',
        };
        final request = _client == null
            ? http.post(url, headers: headers, body: jsonEncode(payload))
            : _client.post(url, headers: headers, body: jsonEncode(payload));
        return request.timeout(const Duration(seconds: 15));
      });
    } catch (_) {
      return const CalendarConnectResult.error(
        'No se pudo conectar con Google Calendar. Inténtalo nuevamente más tarde.',
      );
    }
    if (revision != SessionManager.revision.value) {
      return const CalendarConnectResult.error(
        'La sesión cambió. Vuelve a iniciar la conexión con Calendar.',
      );
    }
    if (resp.statusCode == 200) {
      try {
        final body = jsonDecode(utf8.decode(resp.bodyBytes));
        if (body is Map && body['connected'] == true) {
          return const CalendarConnectResult.ok();
        }
      } catch (_) {}
      return const CalendarConnectResult.error(
        'No se pudo confirmar la conexión con Google Calendar.',
      );
    }
    String? code;
    try {
      final body = jsonDecode(resp.body);
      if (body is Map) {
        final err = body['error'];
        if (err is Map) code = err['code']?.toString();
      }
    } catch (_) {}
    switch (resp.statusCode) {
      case 400 when code == 'invalid_oauth_code':
        return const CalendarConnectResult.error(
          'El código de autorización no es válido o ya venció. Vuelve a intentarlo.',
        );
      case 400:
        return const CalendarConnectResult.error(
          'No se pudo completar la conexión. Inténtalo nuevamente.',
        );
      case 401:
        return const CalendarConnectResult.error(
          'Tu sesión venció. Vuelve a iniciar sesión e inténtalo de nuevo.',
        );
      case 502:
        return const CalendarConnectResult.error(
          'No se pudo conectar con Google. Inténtalo nuevamente más tarde.',
        );
      default:
        return const CalendarConnectResult.error(
          'No se pudo conectar con Google Calendar. Inténtalo nuevamente más tarde.',
        );
    }
  }

  /// Consulta GET /auth/google-calendar/status con la sesión actual.
  /// null = sin sesión o sin red (la UI mantiene su estado previo).
  /// El aislamiento es por cuenta: el backend resuelve por user_id del JWT,
  /// igual que Drive (verificación A/B pendiente en informe manual).
  Future<CalendarStatus?> fetchStatus({required String backendBaseUrl}) async {
    final revision = SessionManager.revision.value;
    if (SessionManager.token?.isNotEmpty != true) return null;
    final base = backendBaseUrl.trim().replaceFirst(RegExp(r'/+$'), '');
    if (base.isEmpty) return null;
    try {
      final resp = await AuthedHttp.run(() {
        if (revision != SessionManager.revision.value) {
          throw StateError('La sesión cambió.');
        }
        final token = SessionManager.token;
        if (token == null || token.isEmpty) {
          throw StateError('No hay una sesión activa.');
        }
        final url = Uri.parse('$base/auth/google-calendar/status');
        final headers = {'Authorization': 'Bearer $token'};
        final request = _client == null
            ? http.get(url, headers: headers)
            : _client.get(url, headers: headers);
        return request.timeout(const Duration(seconds: 10));
      });
      if (revision != SessionManager.revision.value || resp.statusCode != 200) {
        return null;
      }
      final body = jsonDecode(utf8.decode(resp.bodyBytes));
      if (body is! Map || body['connected'] is! bool) return null;
      final email = (body['external_email'] ?? '').toString().trim();
      return CalendarStatus(
        connected: body['connected'] == true,
        externalEmail: email.isEmpty ? null : email,
      );
    } catch (_) {
      return null;
    }
  }

  Future<void> signOut() => _googleSignIn.signOut();
}
