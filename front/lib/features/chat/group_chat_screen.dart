import 'dart:async';

import 'package:flutter/material.dart';

import '../../core/models/social_models.dart';
import '../../core/services/chat_service.dart';
import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';

// Chat grupal con Stream: token real de GET /groups/:id/stream-token,
// conexión directa con el SDK y mensajes en vivo. Sin conversación
// inventada: sin conexión solo hay estado contextual con reintento.
class GroupChatScreen extends StatefulWidget {
  const GroupChatScreen({
    super.key,
    this.groupId,
    SocialService? service,
    StreamChatConnector? connector,
  }) : _serviceOverride = service,
       _connectorOverride = connector;

  final String? groupId;
  final SocialService? _serviceOverride;
  final StreamChatConnector? _connectorOverride;

  @override
  State<GroupChatScreen> createState() => _GroupChatScreenState();
}

enum _ChatPhase { loading, live, unavailable }

class _GroupChatScreenState extends State<GroupChatScreen> {
  late final SocialService _social;
  late StreamChatConnector _connector;
  final TextEditingController _controller = TextEditingController();
  final ScrollController _scroll = ScrollController();

  _ChatPhase _phase = _ChatPhase.loading;
  String _message = '';
  ChatSession? _session;
  StreamSubscription<List<ChatMessage>>? _subscription;
  List<ChatMessage> _messages = const [];
  bool _sending = false;

  /// Miembros del grupo (userId -> nombre real) para identidad de mensajes
  /// existentes. Una sola lectura best-effort, sin N+1.
  Map<String, String> _namesByUserId = const {};

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
    _connector = widget._connectorOverride ?? _buildConnector();
    _connect();
  }

  /// Conector real con display_name de la sesión y resolución por miembros.
  StreamChatConnector _buildConnector() {
    final display = ProfileService.current.value?.displayName.trim() ?? '';
    return StreamChatConnector(
      userName: display.isEmpty ? null : display,
      nameResolver: (id) => _namesByUserId[id],
    );
  }

  @override
  void dispose() {
    _subscription?.cancel();
    _session?.close();
    _controller.dispose();
    _scroll.dispose();
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(GroupChatScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    // Cambio de grupo: cerrar la sesión anterior y conectar de nuevo.
    if (oldWidget.groupId?.trim() != widget.groupId?.trim()) {
      _subscription?.cancel();
      _subscription = null;
      _session?.close();
      _session = null;
      _messages = const [];
      _namesByUserId = const {};
      if (widget._connectorOverride == null) {
        _connector = _buildConnector();
      }
      if (mounted) setState(() {});
      _connect();
    }
  }

  bool get _isRealGroup {
    final id = widget.groupId?.trim() ?? '';
    final uuid = RegExp(
      r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
    );
    return id.isNotEmpty && uuid.hasMatch(id);
  }

  Future<void> _connect() async {
    if (!_isRealGroup) {
      if (!mounted) return;
      setState(() {
        _phase = _ChatPhase.unavailable;
        _message = 'Abre el chat desde un grupo para conversar.';
      });
      return;
    }
    if (!mounted) return;
    setState(() {
      _phase = _ChatPhase.loading;
      _message = '';
    });
    try {
      final token = await _social.getStreamToken(widget.groupId!.trim());
      final userId = SessionManager.currentUserId;
      if (userId == null || userId.isEmpty) {
        throw ChatException('Tu sesión venció. Vuelve a iniciar sesión.');
      }
      final session = await _connector.connect(
        apiKey: token.apiKey ?? '',
        userToken: token.token,
        userId: userId,
        channelId: token.channelId,
      );
      if (!mounted) {
        await session.close();
        return;
      }
      await _subscription?.cancel();
      _session = session;
      _subscription = session.messages.listen(
        (list) {
          if (!mounted) return;
          setState(() => _messages = list);
          _scrollToBottom();
        },
        onError: (_) {
          if (!mounted) return;
          setState(() {
            _phase = _ChatPhase.unavailable;
            _message = 'Se perdió la conexión con el chat.';
          });
        },
      );
      setState(() => _phase = _ChatPhase.live);
      // Nombres de los integrantes para mensajes cuyo user de Stream no
      // trae name (best-effort: sin ellos se usa id corto, nunca UUID).
      _loadMemberNames();
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _phase = _ChatPhase.unavailable;
        _message = _humanBackendMessage(e);
      });
    } on ChatException catch (e) {
      if (!mounted) return;
      setState(() {
        _phase = _ChatPhase.unavailable;
        _message = e.message;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _phase = _ChatPhase.unavailable;
        _message = 'El chat no está disponible en este momento.';
      });
    }
  }

  Future<void> _loadMemberNames() async {
    final id = widget.groupId?.trim() ?? '';
    if (id.isEmpty) return;
    try {
      final members = await _social.listMembers(id);
      if (!mounted) return;
      setState(() {
        _namesByUserId = {for (final m in members) m.userId: m.displayLabel};
      });
    } catch (_) {
      // Sin nombres: la UI usa id corto, sin romper el chat.
    }
  }

  String _humanBackendMessage(SocialApiException e) {
    switch (e.statusCode) {
      case 401:
        return 'Tu sesión venció. Vuelve a iniciar sesión.';
      case 403:
        return 'No perteneces a este grupo.';
      case 404:
        return 'El grupo ya no existe.';
      case 503:
        return 'El chat no está disponible en este momento.';
      default:
        return 'El chat no está disponible en este momento.';
    }
  }

  void _scrollToBottom() {
    if (!_scroll.hasClients) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) return;
      _scroll.animateTo(
        _scroll.position.maxScrollExtent,
        duration: const Duration(milliseconds: 250),
        curve: Curves.easeOut,
      );
    });
  }

  Future<void> _send() async {
    final text = _controller.text.trim();
    final session = _session;
    if (text.isEmpty || session == null || _sending) return;
    setState(() => _sending = true);
    try {
      await session.send(text);
      if (!mounted) return;
      _controller.clear();
    } on ChatException catch (e) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(e.message)));
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Column(
          children: [
            _buildHeader(isDesktop),
            Expanded(
              child: Container(
                margin: EdgeInsets.symmetric(
                  horizontal: isDesktop ? 24 : 12,
                  vertical: 8,
                ),
                decoration: BoxDecoration(
                  color: AppColors.surface,
                  border: Border.all(
                    color: AppColors.border,
                    width: AppDimens.borderWidth,
                  ),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: const [
                    BoxShadow(
                      color: AppColors.border,
                      offset: Offset(3, 3),
                      blurRadius: 0,
                    ),
                  ],
                ),
                child: _buildBody(),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(bool isDesktop) {
    final live = _phase == _ChatPhase.live;
    return Padding(
      padding: EdgeInsets.symmetric(
        horizontal: isDesktop ? 24 : 16,
        vertical: 12,
      ),
      child: Row(
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'CHAT GRUPAL',
                  style: TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: -0.5,
                  ),
                ),
                Text(
                  'Conversación del grupo',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    color: AppColors.muted,
                  ),
                ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: BoxDecoration(
              color: AppColors.bg,
              border: Border.all(color: AppColors.border, width: 2),
              borderRadius: BorderRadius.circular(20),
            ),
            child: Row(
              children: [
                Icon(
                  Icons.circle,
                  size: 8,
                  color: live ? AppColors.success : AppColors.error,
                ),
                const SizedBox(width: 6),
                Text(
                  live ? 'EN LÍNEA' : 'DESCONECTADO',
                  style: const TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildBody() {
    switch (_phase) {
      case _ChatPhase.loading:
        return const Center(
          child: SizedBox(
            width: 28,
            height: 28,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        );
      case _ChatPhase.unavailable:
        return Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(
                  Icons.forum_outlined,
                  size: 40,
                  color: AppColors.mutedStrong,
                ),
                const SizedBox(height: 12),
                Text(
                  _message.isEmpty
                      ? 'El chat no está disponible en este momento.'
                      : _message,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: AppColors.mutedStrong,
                    height: 1.4,
                  ),
                ),
                const SizedBox(height: 12),
                TextButton(
                  onPressed: _connect,
                  child: const Text('Reintentar'),
                ),
              ],
            ),
          ),
        );
      case _ChatPhase.live:
        return Column(
          children: [
            Expanded(
              child: _messages.isEmpty
                  ? const Center(
                      child: Text(
                        'Aún no hay mensajes. Escribe el primero.',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w600,
                          color: AppColors.mutedStrong,
                        ),
                      ),
                    )
                  : ListView.builder(
                      controller: _scroll,
                      padding: const EdgeInsets.all(12),
                      itemCount: _messages.length,
                      itemBuilder: (context, i) =>
                          _MessageBubble(message: _messages[i]),
                    ),
            ),
            const Divider(color: AppColors.border, thickness: 2, height: 1),
            _buildInput(),
          ],
        );
    }
  }

  Widget _buildInput() {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        children: [
          Expanded(
            child: Container(
              decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(24),
              ),
              child: TextField(
                controller: _controller,
                textInputAction: TextInputAction.send,
                onSubmitted: (_) => _send(),
                decoration: const InputDecoration(
                  hintText: 'Escribe un mensaje...',
                  hintStyle: TextStyle(
                    color: AppColors.muted,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                  border: InputBorder.none,
                  contentPadding: EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 12,
                  ),
                ),
                style: const TextStyle(
                  fontWeight: FontWeight.w600,
                  fontSize: 13,
                  color: AppColors.text,
                ),
              ),
            ),
          ),
          const SizedBox(width: 8),
          InkWell(
            onTap: _sending ? null : _send,
            borderRadius: BorderRadius.circular(24),
            child: Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: AppColors.accentYellow,
                border: Border.all(color: AppColors.border, width: 2),
                shape: BoxShape.circle,
              ),
              child: _sending
                  ? const Padding(
                      padding: EdgeInsets.all(12),
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(
                      Icons.send_rounded,
                      size: 18,
                      color: AppColors.text,
                    ),
            ),
          ),
        ],
      ),
    );
  }
}

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.message});

  final ChatMessage message;

  static String _initialOf(String name) {
    final t = name.trim();
    if (t.isEmpty) return '?';
    final parts = t.split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
    if (parts.length >= 2) {
      return '${parts[0][0]}${parts[1][0]}'.toUpperCase();
    }
    final clean = parts.first.replaceAll(
      RegExp(r'[^A-Za-zÁÉÍÓÚÑáéíóúñ0-9]'),
      '',
    );
    if (clean.length >= 2) return clean.substring(0, 2).toUpperCase();
    return t.substring(0, 1).toUpperCase();
  }

  static String _timeOf(DateTime when) {
    final l = when.toLocal();
    return '${l.hour.toString().padLeft(2, '0')}:${l.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final mine = message.isMine;
    // Identidad consistente: "Tú" para propios, display_name real ajeno.
    final who = mine ? 'Tú' : message.authorName;
    return Align(
      alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 4),
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        constraints: BoxConstraints(
          maxWidth: MediaQuery.of(context).size.width * 0.7,
        ),
        decoration: BoxDecoration(
          color: mine ? AppColors.accentYellow : AppColors.bg,
          border: Border.all(color: AppColors.border, width: 2),
          borderRadius: BorderRadius.circular(16),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          mainAxisSize: MainAxisSize.min,
          children: [
            Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Container(
                  width: 22,
                  height: 22,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: mine ? AppColors.border : AppColors.accentYellow,
                    border: Border.all(color: AppColors.border, width: 1.5),
                    borderRadius: BorderRadius.circular(11),
                  ),
                  child: Text(
                    _initialOf(who),
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w900,
                      color: mine ? AppColors.surface : AppColors.text,
                    ),
                  ),
                ),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    who,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.mutedStrong,
                    ),
                  ),
                ),
                const SizedBox(width: 6),
                Text(
                  _timeOf(message.createdAt),
                  style: const TextStyle(
                    fontSize: 9,
                    fontWeight: FontWeight.w600,
                    color: AppColors.muted,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              message.text,
              style: const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: AppColors.text,
                height: 1.35,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
