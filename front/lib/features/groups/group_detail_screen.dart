import 'package:flutter/material.dart';

import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../chat/group_chat_screen.dart';
import '../workspace/kanban_screen.dart';
import '../workspace/schedule_meeting_screen.dart';
import '../workspace/sprint_sheet_screen.dart';

class GroupDetailScreen extends StatefulWidget {
  final Map<String, dynamic> group;

  const GroupDetailScreen({super.key, required this.group, SocialService? service})
      : _serviceOverride = service;

  final SocialService? _serviceOverride;

  @override
  State<GroupDetailScreen> createState() => _GroupDetailScreenState();
}

class _GroupDetailScreenState extends State<GroupDetailScreen> with SingleTickerProviderStateMixin {
  late TabController _tabController;
  int _currentIndex = 0;

  final List<String> _tabs = const ['CHAT', 'DISCORD', 'HOJA SPRINT', 'KANBAN', 'AGENDAR'];

  late final SocialService _service;
  bool _loadingHeader = false;
  String? _headerError;
  GroupDetail? _detail;
  String? _loadedForId;

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
    _service = widget._serviceOverride ?? SocialService();
    _maybeLoadHeader();
  }

  @override
  void didUpdateWidget(GroupDetailScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.group['id']?.toString() != widget.group['id']?.toString()) {
      _maybeLoadHeader();
    }
  }

  @override
  void dispose() {
    _tabController.dispose();
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  String? _realGroupId() {
    final raw = widget.group['id']?.toString().trim() ?? '';
    // El backend usa UUID (36 con guiones). Los mocks viejos (g1/g2) no son
    // reales: se tratan como sin grupo para no llamar con ID inválido.
    final uuid = RegExp(
        r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$');
    if (uuid.hasMatch(raw)) return raw;
    return null;
  }

  // Cabecera real: GET /groups/:id (GroupView).
  // Ver back/social/internal/handler/http/group_handler.go:119 Get.
  Future<void> _loadHeader() async {
    final id = _realGroupId();
    if (id == null) return;
    if (!mounted) return;
    setState(() {
      _loadingHeader = true;
      _headerError = null;
    });
    try {
      final d = await _service.getGroup(id);
      if (!mounted) return;
      setState(() {
        _detail = d;
        _loadedForId = id;
        _loadingHeader = false;
      });
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() {
        _headerError = e.toString();
        _loadingHeader = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _headerError = 'No se pudo cargar el grupo: $e';
        _loadingHeader = false;
      });
    }
  }

  void _maybeLoadHeader() {
    final id = _realGroupId();
    if (id == null) return;
    if (_loadedForId == id && _detail != null) return;
    _detail = null;
    _loadHeader();
  }

  Widget _buildHeaderTitle(String? groupId) {
    // Sin UUID real: solo datos de navegación, rotulados como no actualizados.
    if (groupId == null) {
      final navName =
          (widget.group['name']?.toString() ?? 'Grupo').toUpperCase();
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(navName,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w900,
                  color: Color(0xFF1A1A1A),
                  letterSpacing: -0.3)),
          const Text('Datos de navegación (no actualizados)',
              style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF555555))),
          const Text('Vista previa local: ID no es UUID real, sin backend',
              style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF8A6D00))),
        ],
      );
    }
    if (_loadingHeader) {
      return const Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('CARGANDO GRUPO REAL...',
              style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w900,
                  color: Color(0xFF1A1A1A),
                  letterSpacing: -0.3)),
          SizedBox(height: 4),
          SizedBox(
              width: 120,
              height: 4,
              child: LinearProgressIndicator(
                  color: Color(0xFF1A1A1A),
                  backgroundColor: Color(0xFFF5F0E8))),
        ],
      );
    }
    if (_headerError != null) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text('NO SE PUDO CARGAR EL GRUPO (REAL)',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w900,
                  color: Color(0xFF1A1A1A))),
          Text(_headerError!,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF555555))),
          InkWell(
            onTap: _loadHeader,
            child: const Text('REINTENTAR',
                style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w900,
                    color: Color(0xFF1A1A1A),
                    decoration: TextDecoration.underline)),
          ),
        ],
      );
    }
    final d = _detail;
    if (d == null) {
      return const Text('GRUPO',
          style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w900,
              color: Color(0xFF1A1A1A)));
    }
    final desc = (d.description == null || d.description!.trim().isEmpty)
        ? 'Sin descripción (real)'
        : d.description!.trim();
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(d.name.toUpperCase(),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
                fontSize: 14,
                fontWeight: FontWeight.w900,
                color: Color(0xFF1A1A1A),
                letterSpacing: -0.3)),
        Text('$desc · ${d.role} (real GET /groups/:id)',
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                color: Color(0xFF555555))),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final groupId = _realGroupId();

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
        title: _buildHeaderTitle(groupId),
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
              children: [
                GroupChatTab(groupId: groupId),
                const GroupDiscordTab(),
                SprintSheetScreen(groupId: groupId),
                KanbanScreen(groupId: groupId),
                ScheduleMeetingScreen(groupId: groupId),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class GroupChatTab extends StatelessWidget {
  const GroupChatTab({super.key, this.groupId});

  final String? groupId;

  @override
  Widget build(BuildContext context) {
    return GroupChatScreen(groupId: groupId);
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
  const GroupSprintTab({super.key, this.groupId});

  final String? groupId;

  @override
  Widget build(BuildContext context) {
    return SprintSheetScreen(groupId: groupId);
  }
}

class GroupKanbanTab extends StatelessWidget {
  const GroupKanbanTab({super.key, this.groupId});

  final String? groupId;

  @override
  Widget build(BuildContext context) {
    return KanbanScreen(groupId: groupId);
  }
}

class GroupScheduleTab extends StatelessWidget {
  const GroupScheduleTab({super.key, this.groupId});

  final String? groupId;

  @override
  Widget build(BuildContext context) {
    return ScheduleMeetingScreen(groupId: groupId);
  }
}
