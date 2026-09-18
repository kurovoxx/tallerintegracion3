import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';

class GroupChatScreen extends StatefulWidget {
  const GroupChatScreen({super.key});

  @override
  State<GroupChatScreen> createState() => _GroupChatScreenState();
}

class _GroupChatScreenState extends State<GroupChatScreen> {
  final TextEditingController _controller = TextEditingController();
  final ScrollController _scroll = ScrollController();
  final List<Map<String, dynamic>> _messages = [
    {'id': '1', 'text': 'Hola equipo, ¿revisaron los apuntes de cálculo?', 'isMe': false, 'user': 'Sofía', 'time': '09:12', 'avatar': 'SF'},
    {'id': '2', 'text': 'Sí, subí el resumen de derivadas a Notas', 'isMe': true, 'user': 'Tú', 'time': '09:14', 'avatar': 'YO'},
    {'id': '3', 'text': 'Genial, lo veo ahora. ¿Nos juntamos mañana?', 'isMe': false, 'user': 'Matías', 'time': '09:15', 'avatar': 'MT'},
    {'id': '4', 'text': 'Perfecto, 10:00 en biblioteca', 'isMe': false, 'user': 'Sofía', 'time': '09:16', 'avatar': 'SF'},
  ];

  void _send() {
    final t = _controller.text.trim();
    if (t.isEmpty) return;
    setState(() {
      _messages.add({'id': DateTime.now().millisecondsSinceEpoch.toString(), 'text': t, 'isMe': true, 'user': 'Tú', 'time': '${DateTime.now().hour}:${DateTime.now().minute.toString().padLeft(2, '0')}', 'avatar': 'YO'});
    });
    _controller.clear();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scroll.hasClients) _scroll.animateTo(_scroll.position.maxScrollExtent + 80, duration: const Duration(milliseconds: 250), curve: Curves.easeOut);
    });
  }

  @override
  void dispose() {
    _controller.dispose();
    _scroll.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Column(
          children: [
            _buildHeader(isDesktop),
            Expanded(
              child: Container(
                margin: EdgeInsets.symmetric(horizontal: isDesktop ? 24 : 12, vertical: 8),
                decoration: BoxDecoration(
                  color: AppColors.surface,
                  border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
                  borderRadius: BorderRadius.circular(AppDimens.radius),
                  boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
                ),
                child: Column(
                  children: [
                    _buildChatInfo(),
                    const Divider(color: AppColors.border, thickness: 2, height: 1),
                    Expanded(child: _buildMessages()),
                    const Divider(color: AppColors.border, thickness: 2, height: 1),
                    _buildInput(),
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
      padding: EdgeInsets.symmetric(horizontal: isDesktop ? 24 : 16, vertical: 12),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('CHAT GRUPAL', style: TextStyle(fontSize: isDesktop ? 22 : 18, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.5)),
                const Text('Cálculo II - Grupo Alpha · 5 miembros', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: AppColors.muted)),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(20)),
            child: const Row(children: [Icon(Icons.circle, size: 8, color: Colors.green), SizedBox(width: 6), Text('EN LÍNEA', style: TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text))]),
          ),
        ],
      ),
    );
  }

  Widget _buildChatInfo() {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        children: [
          SizedBox(
            width: 64,
            height: 28,
            child: Stack(children: [
              for (int i = 0; i < 3; i++)
                Positioned(
                  left: i * 16,
                  child: Container(
                    width: 28,
                    height: 28,
                    alignment: Alignment.center,
                    decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 1.5), shape: BoxShape.circle),
                    child: Text(['SF', 'MT', 'YO'][i], style: const TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: AppColors.text)),
                  ),
                ),
            ]),
          ),
          const SizedBox(width: 8),
          const Text('Sofía, Matías y 3 más', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: AppColors.muted)),
          const Spacer(),
          const Icon(Icons.more_horiz_rounded, color: AppColors.muted),
        ],
      ),
    );
  }

  Widget _buildMessages() {
    return ListView.builder(
      controller: _scroll,
      padding: const EdgeInsets.all(16),
      itemCount: _messages.length,
      itemBuilder: (context, i) {
        final m = _messages[i];
        final isMe = m['isMe'] as bool;
        return Padding(
          padding: const EdgeInsets.only(bottom: 12),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            mainAxisAlignment: isMe ? MainAxisAlignment.end : MainAxisAlignment.start,
            children: [
              if (!isMe) ...[
                Container(
                  width: 32,
                  height: 32,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1.5), shape: BoxShape.circle),
                  child: Text(m['avatar'] as String, style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
                ),
                const SizedBox(width: 8),
              ],
              Flexible(
                child: Column(
                  crossAxisAlignment: isMe ? CrossAxisAlignment.end : CrossAxisAlignment.start,
                  children: [
                    if (!isMe)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 4, left: 4),
                        child: Text('${m['user']} · ${m['time']}', style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: AppColors.muted)),
                      ),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                      decoration: BoxDecoration(
                        color: isMe ? AppColors.border : AppColors.bg,
                        border: Border.all(color: AppColors.border, width: 2),
                        borderRadius: BorderRadius.circular(12),
                        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)],
                      ),
                      child: Column(
                        crossAxisAlignment: isMe ? CrossAxisAlignment.end : CrossAxisAlignment.start,
                        children: [
                          Text(m['text'] as String, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: isMe ? Colors.white : AppColors.text, height: 1.3)),
                          const SizedBox(height: 4),
                          Text(m['time'] as String, style: TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: isMe ? Colors.white70 : AppColors.muted)),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              if (isMe) ...[
                const SizedBox(width: 8),
                Container(
                  width: 32,
                  height: 32,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 1.5), shape: BoxShape.circle),
                  child: Text(m['avatar'] as String, style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
                ),
              ],
            ],
          ),
        );
      },
    );
  }

  Widget _buildInput() {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(
        children: [
          IconButton(onPressed: () {}, icon: Container(padding: const EdgeInsets.all(6), decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(6)), child: const Icon(Icons.attach_file_rounded, size: 18, color: AppColors.text))),
          const SizedBox(width: 8),
          Expanded(
            child: Container(
              decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(24)),
              child: TextField(
                controller: _controller,
                decoration: const InputDecoration(hintText: 'Escribe un mensaje...', hintStyle: TextStyle(color: AppColors.muted, fontWeight: FontWeight.w600, fontSize: 13), border: InputBorder.none, contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 12)),
                style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 13, color: AppColors.text),
                onSubmitted: (_) => _send(),
              ),
            ),
          ),
          const SizedBox(width: 8),
          InkWell(
            onTap: _send,
            borderRadius: BorderRadius.circular(24),
            child: Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 2), shape: BoxShape.circle, boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
              child: const Icon(Icons.send_rounded, size: 18, color: AppColors.text),
            ),
          ),
        ],
      ),
    );
  }
}
