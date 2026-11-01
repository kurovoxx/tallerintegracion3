import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/services/chat_service.dart';
import 'package:taller_integracion_front/core/services/session_manager.dart';
import 'package:taller_integracion_front/core/services/social_service.dart';
import 'package:taller_integracion_front/features/chat/group_chat_screen.dart';
import 'package:taller_integracion_front/features/workspace/schedule_meeting_screen.dart';

const _gid = '11111111-1111-1111-1111-111111111111';

class _FakeSession implements ChatSession {
  _FakeSession({List<ChatMessage>? initial})
    : messages = Stream.value(initial ?? const []);

  @override
  final Stream<List<ChatMessage>> messages;

  final sent = <String>[];

  @override
  Future<void> send(String text) async {
    sent.add(text);
  }

  @override
  Future<void> close() async {}
}

SocialService _socialFor(Map<String, dynamic> tokenBody, int tokenStatus) {
  final service = SocialService(
    client: MockClient((request) async {
      return http.Response(
        jsonEncode(tokenBody),
        tokenStatus,
        headers: {'content-type': 'application/json'},
      );
    }),
  );
  addTearDown(service.dispose);
  return service;
}

StreamChatConnector _connectorFor(Future<ChatSession> Function() open) {
  return StreamChatConnector(
    factory:
        ({
          required String apiKey,
          required String userToken,
          required String userId,
          required String channelId,
        }) => open(),
  );
}

Future<void> _pumpChat(
  WidgetTester tester, {
  required SocialService service,
  required StreamChatConnector connector,
}) async {
  tester.view.physicalSize = const Size(1280, 900);
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      home: GroupChatScreen(
        groupId: _gid,
        service: service,
        connector: connector,
      ),
    ),
  );
  await tester.pumpAndSettle();
}

const _tokenBody = {
  'token': 'tok-abc',
  'channel_id': 'group-canal-1',
  'api_key': 'key-abc',
};

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets('chat en vivo muestra mensajes y permite enviar', (tester) async {
    // JWT con user_id para la conexión.
    final payload = base64Url.encode(
      utf8.encode(jsonEncode({'user_id': 'u1', 'exp': 9999999999})),
    );
    await SessionManager.saveSession('h.$payload.f', {'id': 'u1'});
    addTearDown(SessionManager.clear);

    final session = _FakeSession(
      initial: [
        ChatMessage(
          id: 'm1',
          text: 'Hola equipo',
          authorId: 'u2',
          authorName: 'Compañera',
          createdAt: DateTime.now(),
          isMine: false,
        ),
      ],
    );
    await _pumpChat(
      tester,
      service: _socialFor(_tokenBody, 200),
      connector: _connectorFor(() async => session),
    );

    expect(find.text('Hola equipo'), findsOneWidget);
    expect(find.text('Compañera'), findsOneWidget);
    expect(find.text('EN LÍNEA'), findsOneWidget);
    // Sin diagnósticos internos ni conversación inventada.
    expect(find.textContaining('stream-token'), findsNothing);
    expect(find.textContaining('view_handler.go'), findsNothing);
    expect(find.textContaining('Evidencia'), findsNothing);
    expect(find.textContaining('11111111'), findsNothing);

    await tester.enterText(find.byType(TextField), 'Enterado');
    await tester.tap(find.byIcon(Icons.send_rounded));
    await tester.pumpAndSettle();
    expect(session.sent, ['Enterado']);
    expect(tester.takeException(), isNull);
  });

  testWidgets('fallo del SDK al leer canal muestra estado humano', (
    tester,
  ) async {
    // Caso cuenta B: token OK pero Channel.query del SDK falla (403 code 17
    // por falta de membresía en Stream). No debe quedar excepción sin
    // controlar: la pantalla muestra estado humano con reintento.
    final payload = base64Url.encode(
      utf8.encode(jsonEncode({'user_id': 'u1', 'exp': 9999999999})),
    );
    await SessionManager.saveSession('h.$payload.f', {'id': 'u1'});
    addTearDown(SessionManager.clear);

    await _pumpChat(
      tester,
      service: _socialFor(_tokenBody, 200),
      connector: StreamChatConnector(
        factory:
            ({
              required String apiKey,
              required String userToken,
              required String userId,
              required String channelId,
            }) => throw Exception(
              "403 code 17: not allowed ReadChannel in scope 'messaging'",
            ),
      ),
    );

    expect(
      find.text('El chat no está disponible en este momento.'),
      findsOneWidget,
    );
    expect(find.text('DESCONECTADO'), findsOneWidget);
    expect(find.text('Reintentar'), findsOneWidget);
    expect(find.textContaining('403'), findsNothing);
    expect(find.textContaining('code 17'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('chat con 403 muestra mensaje humano', (tester) async {
    final payload = base64Url.encode(
      utf8.encode(jsonEncode({'user_id': 'u1', 'exp': 9999999999})),
    );
    await SessionManager.saveSession('h.$payload.f', {'id': 'u1'});
    addTearDown(SessionManager.clear);

    await _pumpChat(
      tester,
      service: _socialFor({
        'error': 'you must be a member of the group',
        'code': 'forbidden',
      }, 403),
      connector: _connectorFor(() async => _FakeSession()),
    );

    expect(find.text('No perteneces a este grupo.'), findsOneWidget);
    expect(find.text('DESCONECTADO'), findsOneWidget);
    expect(find.textContaining('403'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('agendar sin grupo no envía y lo dice', (tester) async {
    tester.view.physicalSize = const Size(1280, 900);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(const MaterialApp(home: ScheduleMeetingScreen()));
    await tester.pumpAndSettle();

    expect(
      find.text('Selecciona un grupo para agendar una reunión.'),
      findsOneWidget,
    );
    expect(find.text('Vincular con Google Calendar'), findsNothing);
    expect(find.textContaining('POST real'), findsNothing);
    expect(find.textContaining('Sofía'), findsNothing);
    expect(tester.takeException(), isNull);
  });
}
