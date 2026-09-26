import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';

// Chat grupal BLOQUEADO: no existe GET /groups/{id}/stream-token en el backend.
// Evidencia:
// - agentApiContract.md §5:89 promete GET /groups/{id}/stream-token,
// - back/social/internal/handler/http/view_handler.go:80 solo referencia el
//   string "/groups/{id}/stream-token" como token_endpoint,
// - grep stream-token solo halla esa referencia + view_handler_test.go:186,
// - back/social/cmd/server/main.go:188-199 no registra ruta de stream-token,
//   no hay StreamHandler.
// Por eso no se muestran mensajes demo como conversación real ni se simulan
// envíos exitosos.

class GroupChatScreen extends StatefulWidget {
  const GroupChatScreen({super.key, this.groupId});

  final String? groupId;

  @override
  State<GroupChatScreen> createState() => _GroupChatScreenState();
}

class _GroupChatScreenState extends State<GroupChatScreen> {
  final TextEditingController _controller = TextEditingController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _blockedSend() {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text(
            'Mensajería no disponible: falta GET /groups/{id}/stream-token en backend. No se envió nada.'),
        backgroundColor: AppColors.error,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    final gid = widget.groupId?.trim() ?? '';
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Column(
          children: [
            _buildHeader(isDesktop),
            Expanded(
              child: Container(
                margin: EdgeInsets.symmetric(
                    horizontal: isDesktop ? 24 : 12, vertical: 8),
                decoration: BoxDecoration(
                  color: AppColors.surface,
                  border: Border.all(
                      color: AppColors.border, width: AppDimens.borderWidth),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: const [
                    BoxShadow(
                        color: AppColors.border,
                        offset: Offset(3, 3),
                        blurRadius: 0)
                  ],
                ),
                child: Column(
                  children: [
                    Expanded(
                      child: SingleChildScrollView(
                        padding: const EdgeInsets.all(16),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            const Icon(Icons.forum_outlined,
                                size: 40, color: AppColors.mutedStrong),
                            const SizedBox(height: 12),
                            const Text('MENSAJERÍA NO DISPONIBLE',
                                textAlign: TextAlign.center,
                                style: TextStyle(
                                    fontWeight: FontWeight.w900,
                                    fontSize: 14,
                                    color: AppColors.text)),
                            const SizedBox(height: 8),
                            Text(
                              gid.isEmpty
                                  ? 'Abre el chat desde un grupo real. Aun así, el backend no expone GET /groups/{id}/stream-token, por lo que no se puede obtener token ni channel_id reales.'
                                  : 'Grupo $gid: el backend no expone GET /groups/$gid/stream-token, por lo que no se puede obtener token ni channel_id reales.',
                              textAlign: TextAlign.center,
                              style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w600,
                                  color: AppColors.mutedStrong,
                                  height: 1.4),
                            ),
                            const SizedBox(height: 8),
                            const Text(
                              'Evidencia: view_handler.go:80 referencia token_endpoint sin ruta registrada en main.go:188-199; grep stream-token sin handler.',
                              textAlign: TextAlign.center,
                              style: TextStyle(
                                  fontSize: 11,
                                  fontWeight: FontWeight.w600,
                                  color: AppColors.muted),
                            ),
                          ],
                        ),
                      ),
                    ),
                    const Divider(
                        color: AppColors.border, thickness: 2, height: 1),
                    _buildBlockedInput(),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(bool isDesktop) {
    return Padding(
      padding:
          EdgeInsets.symmetric(horizontal: isDesktop ? 24 : 16, vertical: 12),
      child: Row(
        children: [
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('CHAT GRUPAL',
                    style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w900,
                        color: AppColors.text,
                        letterSpacing: -0.5)),
                Text('Bloqueado: sin endpoint de token',
                    style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: AppColors.muted)),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(20)),
            child: const Row(children: [
              Icon(Icons.circle, size: 8, color: AppColors.error),
              SizedBox(width: 6),
              Text('NO DISPONIBLE',
                  style: TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text))
            ]),
          ),
        ],
      ),
    );
  }

  Widget _buildBlockedInput() {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        children: [
          Expanded(
            child: Container(
              decoration: BoxDecoration(
                  color: AppColors.bg,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(24)),
              child: TextField(
                controller: _controller,
                enabled: false,
                decoration: const InputDecoration(
                    hintText: 'Chat deshabilitado (sin backend)...',
                    hintStyle: TextStyle(
                        color: AppColors.muted,
                        fontWeight: FontWeight.w600,
                        fontSize: 13),
                    border: InputBorder.none,
                    contentPadding:
                        EdgeInsets.symmetric(horizontal: 16, vertical: 12)),
                style: const TextStyle(
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                    color: AppColors.text),
              ),
            ),
          ),
          const SizedBox(width: 8),
          InkWell(
            onTap: _blockedSend,
            borderRadius: BorderRadius.circular(24),
            child: Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                  color: AppColors.bg,
                  border: Border.all(color: AppColors.border, width: 2),
                  shape: BoxShape.circle),
              child: const Icon(Icons.block_rounded,
                  size: 18, color: AppColors.mutedStrong),
            ),
          ),
        ],
      ),
    );
  }
}
