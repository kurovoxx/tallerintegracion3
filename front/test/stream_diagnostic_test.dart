import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/services/chat_service.dart';

void main() {
  test('diagnóstico: key inválida falla con ChatException humana', () async {
    final connector = StreamChatConnector();
    Object? caught;
    try {
      await connector.connect(
        apiKey: 'key-invalida-de-prueba',
        userToken: 'token-invalido-de-prueba',
        userId: 'usuario-prueba',
        channelId: 'canal-prueba',
      );
    } catch (e) {
      caught = e;
    }
    // No debe escapar la excepción cruda del SDK ni colgar.
    expect(caught, isA<ChatException>());
    expect(
      (caught as ChatException).message,
      'No se pudo conectar al chat. Inténtalo más tarde.',
    );
  }, timeout: const Timeout(Duration(seconds: 90)));

  test('diagnóstico: sin api key no hay llamada de red', () async {
    final connector = StreamChatConnector(
      factory: ({
        required String apiKey,
        required String userToken,
        required String userId,
        required String channelId,
      }) {
        throw StateError('no debe llamarse sin key');
      },
    );
    Object? caught;
    try {
      await connector.connect(
        apiKey: '  ',
        userToken: 't',
        userId: 'u',
        channelId: 'c',
      );
    } catch (e) {
      caught = e;
    }
    expect(caught, isA<ChatException>());
  });
}
