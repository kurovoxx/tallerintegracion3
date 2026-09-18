import 'package:flutter/material.dart';

import '../chat/group_chat_screen.dart';
import '../workspace/kanban_screen.dart';
import '../workspace/schedule_meeting_screen.dart';
import '../workspace/sprint_sheet_screen.dart';

class GroupDetailScreen extends StatefulWidget {
  final Map<String, dynamic> group;

  const GroupDetailScreen({super.key, required this.group});

  @override
  State<GroupDetailScreen> createState() => _GroupDetailScreenState();
}

class _GroupDetailScreenState extends State<GroupDetailScreen> with SingleTickerProviderStateMixin {
  late TabController _tabController;
  int _currentIndex = 0;

  final List<String> _tabs = const ['CHAT', 'DISCORD', 'HOJA SPRINT', 'KANBAN', 'AGENDAR'];

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 5, vsync: this);
    _tabController.addListener(() {
      if (_tabController.indexIsChanging) {
        setState(() => _currentIndex = _tabController.index);
      } else if (_tabController.index != _currentIndex) {
        setState(() => _currentIndex = _tabController.index);
      }
    });
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final groupName = widget.group['name'] as String? ?? 'Grupo';
    final subject = widget.group['subject'] as String? ?? 'General';
    final members = widget.group['members'] as int? ?? 0;

    return Scaffold(
      backgroundColor: const Color(0xFFF5F0E8),
      appBar: AppBar(
        backgroundColor: Colors.white,
        elevation: 0,
        scrolledUnderElevation: 0,
        shape: const Border(bottom: BorderSide(color: Color(0xFF1A1A1A), width: 2)),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_rounded, color: Color(0xFF1A1A1A)),
          onPressed: () => Navigator.of(context).pop(),
        ),
        title: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(groupName.toUpperCase(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w900, color: Color(0xFF1A1A1A), letterSpacing: -0.3)),
            Text('$subject · $members integrantes',
                style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: Color(0xFF555555))),
          ],
        ),
      ),
      body: Column(
        children: [
          // Barra de pestañas neobrutalista horizontally scrollable
          Container(
            color: Colors.white,
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: List.generate(_tabs.length, (i) {
                  final selected = i == _currentIndex;
                  IconData icon;
                  switch (i) {
                    case 0:
                      icon = Icons.chat_bubble_rounded;
                      break;
                    case 1:
                      icon = Icons.forum_rounded;
                      break;
                    case 2:
                      icon = Icons.assignment_rounded;
                      break;
                    case 3:
                      icon = Icons.view_kanban_rounded;
                      break;
                    case 4:
                      icon = Icons.event_rounded;
                      break;
                    default:
                      icon = Icons.circle;
                  }
                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: GestureDetector(
                      onTap: () {
                        _tabController.animateTo(i);
                        setState(() => _currentIndex = i);
                      },
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                        decoration: BoxDecoration(
                          color: selected ? const Color(0xFFFFCC00) : Colors.white,
                          border: Border.all(color: const Color(0xFF1A1A1A), width: 2),
                          boxShadow: selected
                              ? const [BoxShadow(color: Color(0xFF1A1A1A), offset: Offset(3, 3), blurRadius: 0)]
                              : const [BoxShadow(color: Color(0xFF1A1A1A), offset: Offset(2, 2), blurRadius: 0)],
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(icon, size: 14, color: Colors.black),
                            const SizedBox(width: 6),
                            Text(_tabs[i], style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w900, color: Colors.black, letterSpacing: 0.3)),
                          ],
                        ),
                      ),
                    ),
                  );
                }),
              ),
            ),
          ),
          Container(height: 2, color: Colors.black),
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: const [
                GroupChatTab(),
                GroupDiscordTab(),
                SprintSheetScreen(),
                KanbanScreen(),
                ScheduleMeetingScreen(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class GroupChatTab extends StatelessWidget {
  const GroupChatTab({super.key});

  @override
  Widget build(BuildContext context) {
    return const GroupChatScreen();
  }
}

class GroupDiscordTab extends StatefulWidget {
  const GroupDiscordTab({super.key});

  @override
  State<GroupDiscordTab> createState() => _GroupDiscordTabState();
}

class _GroupDiscordTabState extends State<GroupDiscordTab> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    return Container(
      color: const Color(0xFFF5F0E8),
      child: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // Recuadro Discord #5865F2 con ícono y sombra rígida (discord-link-view)
              GestureDetector(
                onTapDown: (_) => setState(() => _pressed = true),
                onTapUp: (_) => setState(() => _pressed = false),
                onTapCancel: () => setState(() => _pressed = false),
                onTap: () {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(
                      content: Text('Abriendo Discord...', style: TextStyle(color: Colors.white, fontWeight: FontWeight.w800)),
                      backgroundColor: Color(0xFF5865F2),
                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.zero, side: BorderSide(color: Colors.black, width: 2)),
                    ),
                  );
                },
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 80),
                  transform: Matrix4.translationValues(_pressed ? 2 : 0, _pressed ? 2 : 0, 0),
                  padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 20),
                  decoration: BoxDecoration(
                    color: const Color(0xFF5865F2),
                    border: Border.all(color: Colors.black, width: 3),
                    boxShadow: _pressed
                        ? const []
                        : const [BoxShadow(color: Colors.black, offset: Offset(6, 6), blurRadius: 0)],
                  ),
                  child: Column(
                    children: [
                      Container(
                        width: 64,
                        height: 64,
                        decoration: BoxDecoration(color: Colors.white, border: Border.all(color: Colors.black, width: 2), shape: BoxShape.circle),
                        child: const Icon(Icons.forum_rounded, size: 32, color: Color(0xFF5865F2)),
                      ),
                      const SizedBox(height: 16),
                      const Text('ÚNETE AL DISCORD DEL GRUPO',
                          textAlign: TextAlign.center,
                          style: TextStyle(fontSize: 18, fontWeight: FontWeight.w900, color: Colors.white, letterSpacing: -0.5)),
                      const SizedBox(height: 8),
                      const Text('Proyecto Capstone Frontend usa Discord para reuniones de voz y avisos. Entra con un clic.',
                          textAlign: TextAlign.center, style: TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: Colors.white, height: 1.3)),
                      const SizedBox(height: 20),
                      AnimatedContainer(
                        duration: const Duration(milliseconds: 80),
                        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
                        decoration: BoxDecoration(
                          color: Colors.white,
                          border: Border.all(color: Colors.black, width: 2),
                          boxShadow: _pressed ? null : const [BoxShadow(color: Colors.black, offset: Offset(3, 3), blurRadius: 0)],
                        ),
                        child: const Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(Icons.forum_rounded, size: 18, color: Color(0xFF5865F2)),
                            SizedBox(width: 8),
                            Text('ABRIR DISCORD', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: Color(0xFF5865F2))),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(color: Colors.white, border: Border.all(color: Colors.black, width: 2)),
                child: const Row(
                  children: [
                    Icon(Icons.info_outline_rounded, size: 16, color: Colors.black),
                    SizedBox(width: 8),
                    Expanded(child: Text('Serás redirigido a Discord. Asegúrate de tener la app instalada.', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: Colors.black))),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class GroupSprintTab extends StatelessWidget {
  const GroupSprintTab({super.key});

  @override
  Widget build(BuildContext context) {
    return const SprintSheetScreen();
  }
}

class GroupKanbanTab extends StatelessWidget {
  const GroupKanbanTab({super.key});

  @override
  Widget build(BuildContext context) {
    return const KanbanScreen();
  }
}

class GroupScheduleTab extends StatelessWidget {
  const GroupScheduleTab({super.key});

  @override
  Widget build(BuildContext context) {
    return const ScheduleMeetingScreen();
  }
}
