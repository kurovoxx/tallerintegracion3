import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../chat/group_chat_screen.dart';
import '../workspace/kanban_screen.dart';
import '../workspace/schedule_meeting_screen.dart';
import '../workspace/sprint_sheet_screen.dart';

class GroupDetailScreen extends StatefulWidget {
  final Map<String, dynamic> group;

  const GroupDetailScreen({
    super.key,
    required this.group,
    SocialService? service,
  }) : _serviceOverride = service;

  final SocialService? _serviceOverride;

  @override
  State<GroupDetailScreen> createState() => _GroupDetailScreenState();
}

class _GroupDetailScreenState extends State<GroupDetailScreen>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  int _currentIndex = 0;

  final List<String> _tabs = const [
    'CHAT',
    'DISCORD',
    'HOJA SPRINT',
    'KANBAN',
    'AGENDAR',
  ];

  late final SocialService _service;
  bool _loadingHeader = false;
  String? _headerError;
  GroupDetail? _detail;
  String? _loadedForId;

  /// Conteo real de integrantes: inicia con el de navegación (overview) y
  /// se confirma con GET members. Sin requests extra si ya se conoce.
  int? _memberCount;

  static int? _countOf(Map<String, dynamic> group) {
    final raw = group['members'];
    if (raw is int && raw >= 0) return raw;
    if (raw is num) return raw.toInt();
    return null;
  }

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
    _memberCount = _countOf(widget.group);
    _maybeLoadHeader();
  }

  @override
  void didUpdateWidget(GroupDetailScreen oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.group['id']?.toString() != widget.group['id']?.toString()) {
      _memberCount = _countOf(widget.group);
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
      r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
    );
    if (uuid.hasMatch(raw)) return raw;
    return null;
  }

  // Cabecera del grupo con los datos del backend.
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
      // Conteo real de integrantes (una sola lectura, best-effort).
      try {
        final members = await _service.listMembers(id);
        if (!mounted) return;
        setState(() => _memberCount = members.length);
      } catch (_) {}
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() {
        _headerError = 'No se pudo cargar el grupo.';
        _loadingHeader = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _headerError = 'No se pudo cargar el grupo.';
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
    // Sin UUID: solo datos de navegación.
    if (groupId == null) {
      final navName = (widget.group['name']?.toString() ?? 'Grupo')
          .toUpperCase();
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            navName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w900,
              color: Color(0xFF1A1A1A),
              letterSpacing: -0.3,
            ),
          ),
          const Text(
            'Datos de navegación',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: Color(0xFF555555),
            ),
          ),
        ],
      );
    }
    if (_loadingHeader) {
      return const Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'CARGANDO GRUPO...',
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w900,
              color: Color(0xFF1A1A1A),
              letterSpacing: -0.3,
            ),
          ),
          SizedBox(height: 4),
          SizedBox(
            width: 120,
            height: 4,
            child: LinearProgressIndicator(
              color: Color(0xFF1A1A1A),
              backgroundColor: Color(0xFFF5F0E8),
            ),
          ),
        ],
      );
    }
    if (_headerError != null) {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'NO SE PUDO CARGAR EL GRUPO',
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w900,
              color: Color(0xFF1A1A1A),
            ),
          ),
          Text(
            _headerError!,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w700,
              color: Color(0xFF555555),
            ),
          ),
          InkWell(
            onTap: _loadHeader,
            child: const Text(
              'REINTENTAR',
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w900,
                color: Color(0xFF1A1A1A),
                decoration: TextDecoration.underline,
              ),
            ),
          ),
        ],
      );
    }
    final d = _detail;
    if (d == null) {
      return const Text(
        'GRUPO',
        style: TextStyle(
          fontSize: 14,
          fontWeight: FontWeight.w900,
          color: Color(0xFF1A1A1A),
        ),
      );
    }
    // Cabecera: nombre + conteo real. Descripción y rol viven en INFO.
    final count = _memberCount;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          d.name.toUpperCase(),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w900,
            color: Color(0xFF1A1A1A),
            letterSpacing: -0.3,
          ),
        ),
        if (count != null)
          Text(
            '$count ${count == 1 ? 'integrante' : 'integrantes'}',
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w700,
              color: Color(0xFF555555),
            ),
          ),
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
        shape: const Border(
          bottom: BorderSide(color: Color(0xFF1A1A1A), width: 2),
        ),
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
                        padding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 8,
                        ),
                        decoration: BoxDecoration(
                          color: selected
                              ? const Color(0xFFFFCC00)
                              : Colors.white,
                          border: Border.all(
                            color: const Color(0xFF1A1A1A),
                            width: 2,
                          ),
                          boxShadow: selected
                              ? const [
                                  BoxShadow(
                                    color: Color(0xFF1A1A1A),
                                    offset: Offset(3, 3),
                                    blurRadius: 0,
                                  ),
                                ]
                              : const [
                                  BoxShadow(
                                    color: Color(0xFF1A1A1A),
                                    offset: Offset(2, 2),
                                    blurRadius: 0,
                                  ),
                                ],
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(icon, size: 14, color: Colors.black),
                            const SizedBox(width: 6),
                            Text(
                              _tabs[i],
                              style: const TextStyle(
                                fontSize: 11,
                                fontWeight: FontWeight.w900,
                                color: Colors.black,
                                letterSpacing: 0.3,
                              ),
                            ),
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
                GroupChatTab(groupId: groupId, service: _service),
                GroupDiscordTab(groupId: groupId, service: _service),
                SprintSheetScreen(groupId: groupId, service: _service),
                KanbanScreen(groupId: groupId, service: _service),
                ScheduleMeetingScreen(groupId: groupId, service: _service),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class GroupChatTab extends StatelessWidget {
  const GroupChatTab({super.key, this.groupId, SocialService? service})
    : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

  @override
  Widget build(BuildContext context) {
    return GroupChatScreen(groupId: groupId, service: _serviceOverride);
  }
}

class GroupDiscordTab extends StatefulWidget {
  const GroupDiscordTab({super.key, this.groupId, SocialService? service})
    : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

  @override
  State<GroupDiscordTab> createState() => _GroupDiscordTabState();
}

class _GroupDiscordTabState extends State<GroupDiscordTab> {
  late final SocialService _social;
  bool _loadingRole = false;
  bool _isAdmin = false;
  bool _loadingConfig = false;
  DiscordConfig? _config;

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
    _loadRole();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  bool get _hasGroup {
    final id = widget.groupId?.trim() ?? '';
    return id.isNotEmpty;
  }

  Future<void> _loadRole() async {
    if (!_hasGroup) return;
    if (!mounted) return;
    setState(() {
      _loadingRole = true;
      _loadingConfig = true;
    });
    try {
      final detail = await _social.getGroup(widget.groupId!.trim());
      if (!mounted) return;
      setState(() => _isAdmin = detail.role == 'admin');
    } catch (_) {
      // Sin rol se muestra estado neutro.
    } finally {
      if (mounted) setState(() => _loadingRole = false);
    }
    // Configuración guardada (si existe): habilita ABRIR DISCORD real.
    try {
      final cfg = await _social.getDiscordConfig(widget.groupId!.trim());
      if (!mounted) return;
      setState(() => _config = cfg);
    } catch (_) {
      // Sin config legible se mantiene el estado neutro.
    } finally {
      if (mounted) setState(() => _loadingConfig = false);
    }
  }

  Future<void> _openInvite() async {
    final url = _config?.inviteUrl.trim() ?? '';
    if (url.isEmpty) return;
    final uri = Uri.tryParse(url);
    if (uri == null) return;
    try {
      final opened = await launchUrl(uri, mode: LaunchMode.externalApplication);
      if (!opened && mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('No se pudo abrir la invitación.')),
        );
      }
    } catch (_) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('No se pudo abrir la invitación.')),
      );
    }
  }

  // Solo admin: abre diálogo "Configurar Discord" (invite_url + webhook
  // opcional + nombre si el backend lo usa). No ocupa la pestaña.
  Future<void> _openConfigDialog() async {
    if (!_isAdmin || !_hasGroup) return;
    final serverCtrl = TextEditingController(text: _config?.serverName ?? '');
    final inviteCtrl = TextEditingController(text: _config?.inviteUrl ?? '');
    final webhookCtrl = TextEditingController(text: _config?.webhookUrl ?? '');
    var saving = false;
    final saved = await showNeobrutalistDialog<bool>(
      context: context,
      dialog: StatefulBuilder(
        builder: (ctx, setD) => NeobrutalistDialog(
          title: 'CONFIGURAR DISCORD',
          cancelLabel: 'Cancelar',
          confirmLabel: saving ? 'Guardando...' : 'Guardar',
          closeOnConfirm: false,
          onConfirm: saving
              ? null
              : () async {
                  final server = serverCtrl.text.trim();
                  final invite = inviteCtrl.text.trim();
                  if (server.isEmpty || invite.isEmpty) {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                        content: Text(
                          'Completa el nombre del servidor y la invitación.',
                        ),
                      ),
                    );
                    return;
                  }
                  setD(() => saving = true);
                  try {
                    final webhook = webhookCtrl.text.trim().isEmpty
                        ? null
                        : webhookCtrl.text.trim();
                    await _social.updateDiscordConfig(
                      groupId: widget.groupId!.trim(),
                      serverName: server,
                      inviteUrl: invite,
                      webhookUrl: webhook,
                    );
                    if (!mounted) return;
                    setState(() {
                      _config = DiscordConfig(
                        serverName: server,
                        inviteUrl: invite,
                        webhookUrl: webhook,
                      );
                    });
                    if (ctx.mounted) Navigator.of(ctx).pop(true);
                  } on SocialApiException catch (_) {
                    if (!mounted) return;
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(
                        content: Text('No se pudo guardar la integración.'),
                        backgroundColor: AppColors.error,
                      ),
                    );
                    setD(() => saving = false);
                  }
                },
          content: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const AppFieldLabel('NOMBRE DEL SERVIDOR'),
              const SizedBox(height: 4),
              TextField(
                controller: serverCtrl,
                decoration: appInputDecoration('Nombre del servidor'),
              ),
              const SizedBox(height: 8),
              const AppFieldLabel('ENLACE DE INVITACIÓN'),
              const SizedBox(height: 4),
              TextField(
                controller: inviteCtrl,
                decoration: appInputDecoration('Enlace de invitación'),
              ),
              const SizedBox(height: 8),
              const AppFieldLabel('WEBHOOK (OPCIONAL)'),
              const SizedBox(height: 4),
              TextField(
                controller: webhookCtrl,
                decoration: appInputDecoration('Webhook (opcional)'),
              ),
            ],
          ),
        ),
      ),
    );
    serverCtrl.dispose();
    inviteCtrl.dispose();
    webhookCtrl.dispose();
    if (saved == true && mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Integración guardada.'),
          backgroundColor: AppColors.border,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final hasInvite = (_config?.inviteUrl.trim().isNotEmpty ?? false);
    final serverName = _config?.serverName.trim() ?? '';
    return Container(
      color: const Color(0xFFF5F0E8),
      child: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // Tarjeta histórica: DISCORD DEL GRUPO + explicativo + ABRIR.
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 24,
                  vertical: 20,
                ),
                decoration: BoxDecoration(
                  color: const Color(0xFF5865F2),
                  border: Border.all(color: Colors.black, width: 3),
                  boxShadow: const [
                    BoxShadow(
                      color: Colors.black,
                      offset: Offset(6, 6),
                      blurRadius: 0,
                    ),
                  ],
                ),
                child: Column(
                  children: [
                    Container(
                      width: 64,
                      height: 64,
                      decoration: BoxDecoration(
                        color: Colors.white,
                        border: Border.all(color: Colors.black, width: 2),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.forum_rounded,
                        size: 32,
                        color: Color(0xFF5865F2),
                      ),
                    ),
                    const SizedBox(height: 16),
                    const Text(
                      'DISCORD DEL GRUPO',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w900,
                        color: Colors.white,
                        letterSpacing: -0.5,
                      ),
                    ),
                    const SizedBox(height: 8),
                    if (_loadingRole || _loadingConfig)
                      const Center(
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    else ...[
                      if (serverName.isNotEmpty)
                        Text(
                          serverName,
                          textAlign: TextAlign.center,
                          style: const TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w800,
                            color: Colors.white,
                          ),
                        ),
                      const SizedBox(height: 4),
                      Text(
                        hasInvite
                            ? 'Entra al servidor para voz y avisos de reuniones.'
                            : (_isAdmin
                                  ? 'Aún no hay Discord configurado. Configúralo para habilitar avisos.'
                                  : 'El servidor lo configura el equipo administrador.'),
                        textAlign: TextAlign.center,
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                          height: 1.3,
                        ),
                      ),
                      const SizedBox(height: 20),
                      if (hasInvite)
                        InkWell(
                          onTap: _openInvite,
                          child: Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 20,
                              vertical: 12,
                            ),
                            decoration: BoxDecoration(
                              color: Colors.white,
                              border: Border.all(color: Colors.black, width: 2),
                              boxShadow: const [
                                BoxShadow(
                                  color: Colors.black,
                                  offset: Offset(3, 3),
                                  blurRadius: 0,
                                ),
                              ],
                            ),
                            child: const Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Icon(
                                  Icons.forum_rounded,
                                  size: 18,
                                  color: Color(0xFF5865F2),
                                ),
                                SizedBox(width: 8),
                                Text(
                                  'ABRIR DISCORD',
                                  style: TextStyle(
                                    fontWeight: FontWeight.w900,
                                    fontSize: 12,
                                    color: Color(0xFF5865F2),
                                  ),
                                ),
                              ],
                            ),
                          ),
                        ),
                      // Solo admin: acción secundaria que abre el formulario.
                      if (_isAdmin) ...[
                        const SizedBox(height: 12),
                        OutlinedButton.icon(
                          onPressed: _openConfigDialog,
                          icon: const Icon(
                            Icons.settings_rounded,
                            size: 16,
                            color: Colors.white,
                          ),
                          label: const Text(
                            'Configurar Discord',
                            style: TextStyle(
                              fontWeight: FontWeight.w800,
                              color: Colors.white,
                            ),
                          ),
                          style: OutlinedButton.styleFrom(
                            side: const BorderSide(
                              color: Colors.white,
                              width: 2,
                            ),
                          ),
                        ),
                      ],
                    ],
                  ],
                ),
              ),
              const SizedBox(height: 16),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: Colors.white,
                  border: Border.all(color: Colors.black, width: 2),
                ),
                child: const Row(
                  children: [
                    Icon(
                      Icons.info_outline_rounded,
                      size: 16,
                      color: Colors.black,
                    ),
                    SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        'Serás redirigido a Discord. Usa la invitación del grupo.',
                        style: TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w600,
                          color: Colors.black,
                        ),
                      ),
                    ),
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
