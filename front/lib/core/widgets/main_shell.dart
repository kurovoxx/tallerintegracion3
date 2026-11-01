import 'package:flutter/material.dart';

import '../../features/academic/attendance_screen.dart';
import '../../features/academic/courses_screen.dart';
import '../../features/academic/curriculum_screen.dart';
import '../../features/academic/grade_calculator_screen.dart';
import '../../features/academic/profile_screen.dart';
import '../../features/academic/schedule_screen.dart';
import '../../features/groups/groups_screen.dart';
import '../../features/notes/all_notes_screen.dart';
import '../models/social_models.dart';
import '../services/profile_service.dart';
import '../services/social_service.dart';
import '../theme/app_theme.dart';
import 'neobrutalism.dart';

/// Shell global neobrutalista y responsivo:
/// - expandido (> 1024 dp): sidebar 280/72 dp + canvas centrado.
/// - medio (600-1024 dp): rail colapsado de 72 dp + canvas centrado.
/// - compacto (< 600 dp): drawer + FAB contextual.
///
/// La creación de notas es estrictamente contextual a la pantalla de Notas:
/// en desktop vive en la cabecera de AllNotesScreen y en mobile la aporta el
/// FAB, visible solo cuando la pestaña activa es Notas. El shell no intercepta
/// atajos de teclado: Ctrl+N (Cmd+N) lo gestiona AllNotesScreen localmente.
class MainShell extends StatefulWidget {
  const MainShell({
    super.key,
    this.initialIndex = 5,
    this.userData,
    SocialService? service,
    ProfileService? profileService,
  }) : _serviceOverride = service,
       _profileOverride = profileService;

  final int initialIndex;
  final Map<String, dynamic>? userData;
  final SocialService? _serviceOverride;
  final ProfileService? _profileOverride;

  @override
  State<MainShell> createState() => _MainShellState();
}

class _MainShellState extends State<MainShell> {
  static const int _notesIndex = 5;
  static const int _groupsIndex = 6;

  late int _selectedIndex;
  bool _isCollapsed = false;
  final GlobalKey<ScaffoldState> _scaffoldKey = GlobalKey<ScaffoldState>();
  final GlobalKey<AllNotesScreenState> _notesKey =
      GlobalKey<AllNotesScreenState>();

  late final SocialService _social;
  late final ProfileService _profiles;
  Overview? _overview;
  String? _sidebarError;
  bool _loadingSidebar = false;

  @override
  void initState() {
    super.initState();
    _selectedIndex = widget.initialIndex;
    _social = widget._serviceOverride ?? SocialService();
    _profiles = widget._profileOverride ?? ProfileService();
    // Fuente única reactiva: cualquier GET/PATCH exitoso actualiza el pie
    // sin logout/login.
    ProfileService.current.addListener(_onProfileChanged);
    // Sidebar reactivo: create/join/leave invalidan overview automáticamente.
    SocialService.groupsChanged.addListener(_onGroupsChanged);
    _loadSidebar();
    _loadProfile();
  }

  void _onProfileChanged() {
    if (mounted) setState(() {});
  }

  void _onGroupsChanged() {
    _loadSidebar();
  }

  @override
  void dispose() {
    ProfileService.current.removeListener(_onProfileChanged);
    SocialService.groupsChanged.removeListener(_onGroupsChanged);
    if (widget._serviceOverride == null) _social.dispose();
    if (widget._profileOverride == null) _profiles.dispose();
    super.dispose();
  }

  /// Perfil para el pie del sidebar (nombre + institución/correo reales).
  /// Best-effort: si falla, el pie muestra solo el avatar.
  Future<void> _loadProfile() async {
    try {
      await _profiles.getProfile();
      // El servicio publica en ProfileService.current; el listener repinta.
    } catch (_) {
      // Silencioso: el perfil tiene su propia pantalla con reintento.
    }
  }

  Future<void> _loadSidebar() async {
    if (!mounted) return;
    setState(() {
      _loadingSidebar = true;
      _sidebarError = null;
    });
    try {
      final ov = await _social.getOverview();
      if (!mounted) return;
      setState(() {
        _overview = ov;
        _loadingSidebar = false;
      });
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() {
        _sidebarError = 'No se pudieron cargar tus grupos.';
        _loadingSidebar = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _sidebarError = 'No se pudieron cargar tus grupos.';
        _loadingSidebar = false;
      });
    }
  }

  // Pantallas del shell global: 5 académicas + 3 de espacio general.
  List<Widget> get _pages => <Widget>[
    const CurriculumScreen(),
    const CoursesScreen(),
    const ScheduleScreen(),
    const AttendanceScreen(),
    const GradeCalculatorScreen(),
    AllNotesScreen(key: _notesKey),
    const GroupsScreen(),
    ProfileScreen(userData: widget.userData),
  ];

  List<_NavItem> get _navItems => const <_NavItem>[
    _NavItem(
      icon: Icons.account_tree_rounded,
      label: 'Malla Curricular',
      short: 'Malla',
      section: 'MÓDULO ACADÉMICO',
    ),
    _NavItem(
      icon: Icons.menu_book_rounded,
      label: 'Cursos',
      short: 'Cursos',
      section: 'MÓDULO ACADÉMICO',
    ),
    _NavItem(
      icon: Icons.calendar_month_rounded,
      label: 'Horarios',
      short: 'Horarios',
      section: 'MÓDULO ACADÉMICO',
    ),
    _NavItem(
      icon: Icons.fact_check_rounded,
      label: 'Asistencia',
      section: 'MÓDULO ACADÉMICO',
    ),
    _NavItem(
      icon: Icons.calculate_rounded,
      label: 'Calculadora de Notas',
      section: 'MÓDULO ACADÉMICO',
    ),
    _NavItem(
      icon: Icons.description_rounded,
      label: 'Todas las Notas',
      short: 'Notas',
      section: 'ESPACIO DE TRABAJO GENERAL',
    ),
    _NavItem(
      icon: Icons.groups_rounded,
      label: 'Grupos Académicos',
      section: 'ESPACIO DE TRABAJO GENERAL',
    ),
    _NavItem(
      icon: Icons.person_rounded,
      label: 'Perfil de Usuario',
      section: 'ESPACIO DE TRABAJO GENERAL',
    ),
  ];

  /// Selecciona una sección y cierra el drawer si está abierto.
  void _onSelectPage(int index) {
    final scaffold = _scaffoldKey.currentState;
    if (scaffold != null && scaffold.isDrawerOpen) {
      Navigator.of(context).pop();
    }
    setState(() => _selectedIndex = index);
  }

  /// Acción contextual "Nueva Nota" (solo pestaña de Notas): delega el diálogo
  /// de creación a la pantalla de Notas mediante su GlobalKey.
  void _openNoteDialog() {
    _notesKey.currentState?.openCreateDialog();
  }

  @override
  Widget build(BuildContext context) {
    final breakpoint = context.breakpoint;
    final isDesktop = breakpoint == AppBreakpoint.expanded;
    final isCompact = breakpoint == AppBreakpoint.compact;

    return Scaffold(
      key: _scaffoldKey,
      backgroundColor: AppColors.bg,
      appBar: _buildAppBar(isDesktop: isDesktop),
      drawer: isDesktop ? null : _buildDrawer(),
      // El FAB de creación es contextual: solo en la pestaña de Notas.
      floatingActionButton: isCompact && _selectedIndex == _notesIndex
          ? NeobrutalistFab(tooltip: 'Nueva Nota', onPressed: _openNoteDialog)
          : null,
      body: _buildBody(
        isDesktop: isDesktop,
        showRail: breakpoint == AppBreakpoint.medium,
      ),
    );
  }

  PreferredSizeWidget _buildAppBar({required bool isDesktop}) {
    return AppBar(
      backgroundColor: AppColors.surface,
      elevation: 0,
      scrolledUnderElevation: 0,
      shape: const Border(
        bottom: BorderSide(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
      ),
      leading: Builder(
        builder: (BuildContext innerContext) => IconButton(
          icon: Icon(
            isDesktop
                ? (_isCollapsed ? Icons.menu_rounded : Icons.menu_open_rounded)
                : Icons.menu_rounded,
            color: AppColors.text,
          ),
          tooltip: isDesktop
              ? (_isCollapsed ? 'Expandir menú' : 'Colapsar menú')
              : 'Abrir menú',
          onPressed: () {
            if (isDesktop) {
              setState(() => _isCollapsed = !_isCollapsed);
            } else {
              Scaffold.of(innerContext).openDrawer();
            }
          },
        ),
      ),
      title: Row(
        children: <Widget>[
          Container(
            width: 36,
            height: 36,
            decoration: BoxDecoration(
              color: AppColors.accentYellow,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
            ),
            child: const Center(
              child: Text(
                'S',
                style: TextStyle(
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                ),
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          const Flexible(
            child: Text(
              'SIGMA ACADEMY',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 16,
                color: AppColors.text,
                letterSpacing: -0.5,
              ),
            ),
          ),
        ],
      ),
      actions: const <Widget>[SizedBox(width: AppDimens.spaceSm)],
    );
  }

  Widget _buildBody({required bool isDesktop, required bool showRail}) {
    if (!isDesktop && !showRail) return _buildCanvas();

    final collapsed = isDesktop ? _isCollapsed : true;
    final width = collapsed
        ? AppDimens.sidebarCollapsed
        : AppDimens.sidebarExpanded;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        AnimatedContainer(
          duration: AppMotion.expand,
          curve: AppMotion.shell,
          width: width,
          decoration: const BoxDecoration(
            color: AppColors.surface,
            border: Border(
              right: BorderSide(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
            ),
            // Hard-edge sin blur: el borde de tinta proyecta sombra sólida.
            boxShadow: <BoxShadow>[AppShadows.hardRight],
          ),
          child: _buildSidebarContent(isCollapsed: collapsed),
        ),
        Expanded(child: _buildCanvas()),
      ],
    );
  }

  Widget _buildCanvas() {
    return MaxWidthContainer(
      padding: EdgeInsets.zero,
      child: _pages[_selectedIndex],
    );
  }

  Widget _buildDrawer() {
    return Drawer(
      elevation: 0,
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        side: BorderSide(color: AppColors.border, width: AppDimens.borderWidth),
      ),
      child: SafeArea(child: _buildSidebarContent(isCollapsed: false)),
    );
  }

  Widget _buildSidebarContent({required bool isCollapsed}) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(vertical: AppDimens.spaceSm),
            itemCount: _navItems.length,
            itemBuilder: (BuildContext context, int index) {
              final item = _navItems[index];
              final isSelected = index == _selectedIndex;
              final showSectionHeader =
                  index == 0 || item.section != _navItems[index - 1].section;

              return Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  if (showSectionHeader && !isCollapsed)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(
                        AppDimens.spaceLg,
                        AppDimens.spaceLg,
                        AppDimens.spaceLg,
                        6,
                      ),
                      child: Text(
                        item.section,
                        style: const TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w900,
                          color: AppColors.muted,
                          letterSpacing: 0.8,
                        ),
                      ),
                    ),
                  _SidebarTile(
                    icon: item.icon,
                    label: item.label,
                    isSelected: isSelected,
                    isCollapsed: isCollapsed,
                    onTap: () => _onSelectPage(index),
                  ),
                ],
              );
            },
          ),
        ),
        _buildGroupsFooter(isCollapsed: isCollapsed),
        const Divider(
          color: AppColors.border,
          thickness: AppDimens.borderWidth,
          height: 1,
        ),
        _buildProfileFooter(isCollapsed: isCollapsed),
      ],
    );
  }

  // Barra lateral global: grupos del overview.
  // Ver back/social/internal/handler/http/view_handler.go:29 y model/views.go.
  Widget _buildGroupsFooter({required bool isCollapsed}) {
    if (isCollapsed) {
      final count = _overview?.groupsCount;
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Center(
          child: _loadingSidebar
              ? const SizedBox(
                  width: 18,
                  height: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : InkWell(
                  onTap: () => _onSelectPage(_groupsIndex),
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 4,
                    ),
                    decoration: BoxDecoration(
                      color: AppColors.accentYellow,
                      border: Border.all(
                        color: AppColors.border,
                        width: AppDimens.borderWidth,
                      ),
                      borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                    ),
                    child: Text(
                      '${count ?? '·'}',
                      style: const TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 11,
                      ),
                    ),
                  ),
                ),
        ),
      );
    }
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 4, 12, 8),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            children: [
              const Expanded(
                child: Text(
                  'MIS GRUPOS',
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.6,
                    color: AppColors.muted,
                  ),
                ),
              ),
              InkWell(
                onTap: _loadingSidebar ? null : _loadSidebar,
                child: const Icon(
                  Icons.refresh_rounded,
                  size: 14,
                  color: AppColors.text,
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          if (_loadingSidebar)
            const Center(
              child: SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            )
          else if (_sidebarError != null)
            Text(
              _sidebarError!,
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: AppColors.mutedStrong,
              ),
            )
          else if (_overview == null || _overview!.sidebarGroups.isEmpty)
            const Text(
              'Sin grupos. Crea uno en Grupos.',
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: AppColors.mutedStrong,
              ),
            )
          else ...[
            Builder(
              builder: (context) {
                final o = _overview!;
                return Text(
                  '${o.groupsCount} ${o.groupsCount == 1 ? 'grupo' : 'grupos'} · '
                  '${o.adminGroupsCount} como admin',
                  style: const TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w800,
                    color: AppColors.text,
                  ),
                );
              },
            ),
            const SizedBox(height: 6),
            for (final g in _overview!.sidebarGroups.take(5))
              InkWell(
                onTap: () => _onSelectPage(_groupsIndex),
                child: Padding(
                  padding: const EdgeInsets.symmetric(vertical: 2),
                  child: Row(
                    children: [
                      const Icon(
                        Icons.groups_rounded,
                        size: 13,
                        color: AppColors.text,
                      ),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          g.name,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontSize: 12,
                            fontWeight: FontWeight.w700,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                      Text(
                        g.role,
                        style: const TextStyle(
                          fontSize: 10,
                          fontWeight: FontWeight.w800,
                          color: AppColors.mutedStrong,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        ],
      ),
    );
  }

  Widget _buildProfileFooter({required bool isCollapsed}) {
    // Lee la fuente reactiva única (se actualiza con cada PATCH exitoso).
    final profile = ProfileService.current.value;
    final name = (profile?.displayName.trim().isNotEmpty ?? false)
        ? profile!.displayName.trim()
        : null;
    final second = (profile?.institution?.trim().isNotEmpty ?? false)
        ? profile!.institution!.trim()
        : (profile?.email?.trim().isNotEmpty ?? false)
        ? profile!.email!.trim()
        : null;
    return Padding(
      padding: EdgeInsets.all(
        isCollapsed ? AppDimens.spaceSm : AppDimens.spaceMd,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: <Widget>[
          Row(
            children: <Widget>[
              Container(
                width: 36,
                height: 36,
                decoration: BoxDecoration(
                  color: AppColors.accentYellow,
                  border: Border.all(
                    color: AppColors.border,
                    width: AppDimens.borderWidth,
                  ),
                  borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
                ),
                child: const Icon(
                  Icons.person_rounded,
                  size: 18,
                  color: AppColors.text,
                ),
              ),
              if (!isCollapsed) ...<Widget>[
                const SizedBox(width: AppDimens.spaceSm),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        name ?? 'Mi cuenta',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 12,
                          color: AppColors.text,
                        ),
                      ),
                      if (second != null)
                        Text(
                          second,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                            fontWeight: FontWeight.w600,
                            fontSize: 11,
                            color: AppColors.muted,
                          ),
                        ),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ],
      ),
    );
  }
}

class _NavItem {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.section,
    this.short,
  });

  final IconData icon;
  final String label;
  final String? short;
  final String section;
}

class _SidebarTile extends StatelessWidget {
  const _SidebarTile({
    required this.icon,
    required this.label,
    required this.isSelected,
    required this.isCollapsed,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final bool isSelected;
  final bool isCollapsed;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final tile = ClickCursor(
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: AnimatedContainer(
          duration: AppMotion.medium,
          curve: AppMotion.standard,
          padding: EdgeInsets.symmetric(
            horizontal: isCollapsed ? 0 : AppDimens.spaceMd,
            vertical: 10,
          ),
          decoration: BoxDecoration(
            color: isSelected ? AppColors.accentYellow : Colors.transparent,
            border: Border.all(
              color: isSelected ? AppColors.border : Colors.transparent,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: isSelected ? AppShadows.badge : null,
          ),
          child: Row(
            mainAxisAlignment: isCollapsed
                ? MainAxisAlignment.center
                : MainAxisAlignment.start,
            children: <Widget>[
              Icon(icon, size: 20, color: AppColors.text),
              if (!isCollapsed) ...<Widget>[
                const SizedBox(width: AppDimens.spaceSm),
                Expanded(
                  child: Text(
                    label,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 12.5,
                      fontWeight: isSelected
                          ? FontWeight.w900
                          : FontWeight.w700,
                      color: AppColors.text,
                    ),
                  ),
                ),
              ],
            ],
          ),
        ),
      ),
    );

    return Padding(
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceSm,
        vertical: 3,
      ),
      child: isCollapsed ? Tooltip(message: label, child: tile) : tile,
    );
  }
}
