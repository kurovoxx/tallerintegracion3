import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:stream_chat/stream_chat.dart' as stream;

/// Mensaje del chat grupal en el modelo de la app (sin dependencia de UI).
class ChatMessage {
  ChatMessage({
    required this.id,
    required this.text,
    required this.authorId,
    required this.authorName,
    required this.createdAt,
    required this.isMine,
  });

  final String id;
  final String text;
  final String authorId;
  final String authorName;
  final DateTime createdAt;
  final bool isMine;
}

/// Sesión abierta contra un canal de Stream: mensajes en vivo + envío.
abstract class ChatSession {
  Stream<List<ChatMessage>> get messages;
  Future<void> send(String text);
  Future<void> close();
}

/// Error controlable para mostrar en UI (nunca incluye el token).
class ChatException implements Exception {
  ChatException(this.message);

  final String message;

  @override
  String toString() => message;
}

/// Fábrica de sesiones inyectable (real o fake en tests).
typedef ChatSessionFactory =
    Future<ChatSession> Function({
      required String apiKey,
      required String userToken,
      required String userId,
      required String channelId,
    });

/// Conexión real con Stream Chat (SDK `stream_chat`, Dart puro).
/// Requiere apiKey pública + token de usuario emitidos por el backend
/// (GET /groups/:id/stream-token). El secret jamás sale del backend.
///
/// Identidad: [userName] (display_name real de la sesión) se envía como
/// `User.name` al conectar para que los mensajes nuevos traigan autor.
/// [nameResolver] resuelve autores de mensajes existentes contra los
/// miembros ya cargados del grupo (una sola lectura, sin N+1).
class StreamChatConnector {
  StreamChatConnector({
    ChatSessionFactory? factory,
    String? userName,
    String? Function(String userId)? nameResolver,
  }) : _factory = factory ?? _defaultFactory(userName, nameResolver);

  static ChatSessionFactory _defaultFactory(
    String? userName,
    String? Function(String userId)? nameResolver,
  ) {
    return ({
      required String apiKey,
      required String userToken,
      required String userId,
      required String channelId,
    }) => _openStreamSession(
      apiKey: apiKey,
      userToken: userToken,
      userId: userId,
      channelId: channelId,
      userName: userName,
      nameResolver: nameResolver,
    );
  }

  final ChatSessionFactory _factory;

  Future<ChatSession> connect({
    required String apiKey,
    required String userToken,
    required String userId,
    required String channelId,
  }) {
    if (apiKey.trim().isEmpty) {
      throw ChatException('El chat no está disponible en este momento.');
    }
    return _factory(
      apiKey: apiKey,
      userToken: userToken,
      userId: userId,
      channelId: channelId,
    );
  }

  static Future<ChatSession> _openStreamSession({
    required String apiKey,
    required String userToken,
    required String userId,
    required String channelId,
    String? userName,
    String? Function(String userId)? nameResolver,
  }) async {
    final client = stream.StreamChatClient(apiKey);
    try {
      final name = userName?.trim() ?? '';
      await client
          .connectUser(
            name.isEmpty
                ? stream.User(id: userId)
                : stream.User(id: userId, name: name),
            userToken,
          )
          .timeout(const Duration(seconds: 15));
      final channel = client.channel('messaging', id: channelId);
      await channel.watch().timeout(const Duration(seconds: 15));
      return _StreamChatSession(client, channel, userId, nameResolver);
    } catch (e) {
      debugPrint('[Chat] conexión Stream fallida: ${e.runtimeType}');
      try {
        await client.disconnectUser();
      } catch (_) {}
      throw ChatException('No se pudo conectar al chat. Inténtalo más tarde.');
    }
  }
}

class _StreamChatSession implements ChatSession {
  _StreamChatSession(this._client, this._channel, this._userId, this._names);

  final stream.StreamChatClient _client;
  final stream.Channel _channel;
  final String _userId;

  /// Miembros ya cargados (userId -> nombre real). Sin llamadas extra.
  final String? Function(String userId)? _names;

  @override
  Stream<List<ChatMessage>> get messages {
    List<ChatMessage> mapAll(List<stream.Message> list) {
      final out = <ChatMessage>[];
      for (final m in list) {
        final text = m.text;
        if (text == null || text.trim().isEmpty) continue;
        out.add(_map(m, text.trim()));
      }
      return out;
    }

    final initial = mapAll(_channel.state!.messages);
    final live = _channel.state!.messagesStream.map(mapAll);
    return Stream.multi((controller) {
      controller.add(initial);
      final sub = live.listen(
        controller.add,
        onError: controller.addError,
        onDone: controller.close,
      );
      controller.onCancel = sub.cancel;
    });
  }

  ChatMessage _map(stream.Message m, String text) {
    final author = m.user;
    final authorId = author?.id ?? '';
    final created = m.createdAt;
    return ChatMessage(
      id: m.id.isNotEmpty ? m.id : '${created.millisecondsSinceEpoch}',
      text: text,
      authorId: authorId,
      authorName: _displayName(author?.name ?? '', authorId),
      createdAt: created,
      isMine: authorId == _userId,
    );
  }

  /// Nombre humano del autor: name de Stream, miembros del grupo,
  /// id corto como último recurso. Nunca UUID completo ni vacío.
  String _displayName(String streamName, String authorId) {
    final s = streamName.trim();
    if (s.isNotEmpty) return s;
    final known = _names?.call(authorId)?.trim() ?? '';
    if (known.isNotEmpty) return known;
    if (authorId.isEmpty) return 'Compañero';
    if (authorId == _userId) return 'Tú';
    return authorId.length > 8 ? '${authorId.substring(0, 8)}…' : authorId;
  }

  @override
  Future<void> send(String text) async {
    final content = text.trim();
    if (content.isEmpty) return;
    try {
      await _channel
          .sendMessage(stream.Message(text: content))
          .timeout(const Duration(seconds: 15));
    } catch (_) {
      throw ChatException('No se pudo enviar el mensaje.');
    }
  }

  @override
  Future<void> close() async {
    try {
      await _client.disconnectUser();
    } catch (_) {}
  }
}
