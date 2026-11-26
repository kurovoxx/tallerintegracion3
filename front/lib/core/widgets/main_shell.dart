import 'dart:async';

import 'package:flutter/material.dart';

import '../../features/academic/followed_screen.dart';
import '../../features/academic/grade_calculator_screen.dart';
import '../../features/academic/attendance_screen.dart';
import '../../features/academic/courses_screen.dart';
import '../../features/academic/curriculum_screen.dart';
import '../../features/academic/profile_screen.dart';
import '../../features/academic/schedule_screen.dart';
import '../../features/groups/groups_screen.dart';
import '../../features/notifications/notification_inbox.dart';
import '../../features/notes/all_notes_screen.dart';
import '../models/social_models.dart';
import '../models/profile_models.dart';
import '../services/profile_service.dart';
import '../services/social_service.dart';
import '../theme/app_theme.dart';
import 'color_mode_toggle.dart';
import 'neobrutalism.dart';
import 'user_avatar.dart';
import '../../features/academic/public_profile_screen.dart';

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
  static const int _profileIndex = 7;
  static const int _followedIndex = 8;

  /// true solo cuando se llega al perfil desde la sugerencia de Drive.
  bool _profileScrollToDrive = false;

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
  // Puntito rojo de la campana: se refresca al abrir el shell, al cerrar la
  // bandeja y cada minuto (el backend no expone un contador dedicado).
  int _unreadNotifications = 0;
  bool _loadingUnread = false;
  Timer? _unreadTimer;

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
    _loadUnreadNotifications();
    _unreadTimer = Timer.periodic(
      const Duration(minutes: 1),
      (_) => _loadUnreadNotifications(),
    );
  }

  void _onProfileChanged() {
    if (mounted) setState(() {});
  }

  void _onGroupsChanged() {
    _loadSidebar();
  }

  void _openInbox() async {
    await showNeobrutalistDialog<void>(
      context: context,
      dialog: NotificationInbox(service: _social),
    );
    // Marcar leídas o eliminar dentro del diálogo cambia el puntito.
    if (!mounted) return;
    await _loadUnreadNotifications();
  }

  /// Cuenta notificaciones no leídas. Best-effort: un fallo de red no debe
  /// romper el shell ni mostrar errores en el sidebar.
  Future<void> _loadUnreadNotifications() async {
    if (_loadingUnread) return;
    _loadingUnread = true;
    try {
      final items = await _social.myNotifications();
      final invitations = await _social.listPendingInvitations();
      if (!mounted) return;
      final unread = items.where((n) => !n.read).length + invitations.length;
      if (unread != _unreadNotifications) {
        setState(() => _unreadNotifications = unread);
      }
    } catch (_) {
      // Silencioso: el indicador se reintenta en el próximo ciclo.
    } finally {
      _loadingUnread = false;
    }
  }

  @override
  void dispose() {
    _unreadTimer?.cancel();
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
    AllNotesScreen(
      key: _notesKey,
      onOpenProfile: _openProfileDrive,
      driveStatusChecker: _profiles.getDriveStatus,
    ),
    const GroupsScreen(),
    ProfileScreen(
      userData: widget.userData,
      scrollToDrive: _profileScrollToDrive,
    ),
    const FollowedScreen(),
  ];

  /// «Ir a mi perfil» desde la sugerencia de Drive en Apuntes: abre el perfil
  /// y lo desplaza hasta la tarjeta «Google Drive», donde está «Conectar Drive».
  void _openProfileDrive() {
    setState(() {
      _profileScrollToDrive = true;
      _selectedIndex = _profileIndex;
    });
  }

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
    _NavItem(
      icon: Icons.person_add_rounded,
      label: 'Seguidos',
      section: 'ESPACIO DE TRABAJO GENERAL',
    ),
  ];

  /// Selecciona una sección y cierra el drawer si está abierto.
  void _onSelectPage(int index) {
    final scaffold = _scaffoldKey.currentState;
    if (scaffold != null && scaffold.isDrawerOpen) {
      Navigator.of(context).pop();
    }
    setState(() {
      _profileScrollToDrive = false;
      _selectedIndex = index;
    });
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
      backgroundColor: AppColors.appBarBg,
      foregroundColor: AppColors.appBarFg,
      elevation: 0,
      scrolledUnderElevation: 0,
      shape: Border(
        bottom: BorderSide(
          color: AppColors.appBarBorder,
          width: AppThemeController.isUct ? 3 : AppDimens.borderWidth,
        ),
      ),
      leading: Builder(
        builder: (BuildContext innerContext) => IconButton(
          icon: Icon(
            isDesktop
                ? (_isCollapsed ? Icons.menu_rounded : Icons.menu_open_rounded)
                : Icons.menu_rounded,
            color: AppColors.appBarFg,
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
                color: AppColors.outline,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
            ),
            child: Center(
              child: Image.asset(
                'assets/LogoDentro.png',
                width: 28,
                height: 28,
                fit: BoxFit.cover,
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          Flexible(
            child: Text(
              'SIGMA ACADEMY',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 16,
                color: AppColors.appBarFg,
                letterSpacing: -0.5,
              ),
            ),
          ),
        ],
      ),
      actions: <Widget>[
        // Interruptor de paleta (neobrutalista <-> UCT), como un modo oscuro.
        const ColorModeToggle(),
        IconButton(
          tooltip: 'Notificaciones',
          onPressed: _openInbox,
          icon: Stack(
            clipBehavior: Clip.none,
            children: [
              Icon(Icons.notifications_rounded, color: AppColors.appBarFg),
              if (_unreadNotifications > 0)
                Positioned(
                  right: -3,
                  top: -2,
                  child: Container(
                    key: const ValueKey('notifications-unread-dot'),
                    width: 9,
                    height: 9,
                    decoration: BoxDecoration(
                      color: AppColors.error,
                      shape: BoxShape.circle,
                      border: Border.all(color: AppColors.appBarBg, width: 1.5),
                    ),
                  ),
                ),
            ],
          ),
        ),
        const SizedBox(width: AppDimens.spaceSm),
      ],
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
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border(
              right: BorderSide(
                color: AppColors.outline,
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
      shape: RoundedRectangleBorder(
        side: BorderSide(color: AppColors.outline, width: AppDimens.borderWidth),
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
                        style: TextStyle(
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
        Divider(
          color: AppColors.border,
          thickness: AppDimens.borderWidth,
          height: 1,
        ),
        _buildProfileFooter(isCollapsed: isCollapsed),
      ],
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
              UserAvatar(
                name: name ?? 'Mi cuenta',
                size: 36,
                photoUrl: profile?.photoThumbUrl ?? profile?.photoUrl,
                borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
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
                        style: TextStyle(
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
                          style: TextStyle(
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
    this.showDot = false,
  });

  final IconData icon;
  final String label;
  final bool isSelected;
  final bool isCollapsed;
  final VoidCallback onTap;

  /// Puntito rojo de "hay notificaciones sin leer" sobre el ícono.
  final bool showDot;

  @override
  Widget build(BuildContext context) {
    final uct = AppThemeController.isUct;
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
            color: isSelected ? AppColors.navSelectedBg : Colors.transparent,
            // UCT: barra dorada a la izquierda; Sigma: contorno + sombra dura.
            border: uct
                ? Border(
                    left: BorderSide(
                      color: isSelected
                          ? AppColors.navIndicator
                          : Colors.transparent,
                      width: 4,
                    ),
                    // Mismo alto que en Sigma (borde de 2 px invisible).
                    top: const BorderSide(
                      color: Colors.transparent,
                      width: AppDimens.borderWidth,
                    ),
                    right: const BorderSide(
                      color: Colors.transparent,
                      width: AppDimens.borderWidth,
                    ),
                    bottom: const BorderSide(
                      color: Colors.transparent,
                      width: AppDimens.borderWidth,
                    ),
                  )
                : Border.all(
                    color: isSelected ? AppColors.border : Colors.transparent,
                    width: AppDimens.borderWidth,
                  ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: isSelected && !uct ? AppShadows.badge : null,
          ),
          child: Row(
            mainAxisAlignment: isCollapsed
                ? MainAxisAlignment.center
                : MainAxisAlignment.start,
            children: <Widget>[
              Stack(
                clipBehavior: Clip.none,
                children: <Widget>[
                  Icon(
                    icon,
                    size: 20,
                    color: isSelected ? AppColors.navSelectedFg : AppColors.text,
                  ),
                  if (showDot)
                    Positioned(
                      right: -3,
                      top: -2,
                      child: Container(
                        key: const ValueKey('notifications-unread-dot'),
                        width: 9,
                        height: 9,
                        decoration: BoxDecoration(
                          color: AppColors.error,
                          shape: BoxShape.circle,
                          border: Border.all(
                            color: isSelected
                                ? AppColors.navSelectedBg
                                : AppColors.surface,
                            width: 1.5,
                          ),
                        ),
                      ),
                    ),
                ],
              ),
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
                      color: isSelected
                          ? AppColors.navSelectedFg
                          : AppColors.text,
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
