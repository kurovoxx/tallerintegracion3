import 'package:flutter/material.dart';

import '../../features/academic/attendance_screen.dart';
import '../../features/academic/courses_screen.dart';
import '../../features/academic/curriculum_screen.dart';
import '../../features/academic/grade_calculator_screen.dart';
import '../../features/academic/profile_screen.dart';
import '../../features/academic/schedule_screen.dart';
import '../../features/auth/login_screen.dart';
import '../../features/groups/groups_screen.dart';
import '../../features/notes/all_notes_screen.dart';
import '../services/session_manager.dart';
import '../theme/app_theme.dart';

class MainShell extends StatefulWidget {
  const MainShell({super.key, this.initialIndex = 5, this.userData});

  final int initialIndex;
  final Map<String, dynamic>? userData;

  @override
  State<MainShell> createState() => _MainShellState();
}

class _MainShellState extends State<MainShell> {
  late int _selectedIndex;
  bool _isCollapsed = false;

  @override
  void initState() {
    super.initState();
    _selectedIndex = widget.initialIndex;
  }

  // Pantallas del shell global: 5 académicas + 3 de espacio general
  List<Widget> get _pages => [
        const CurriculumScreen(),
        const CoursesScreen(),
        const ScheduleScreen(),
        const AttendanceScreen(),
        const GradeCalculatorScreen(),
        const AllNotesScreen(),
        const GroupsScreen(),
        ProfileScreen(userData: widget.userData),
      ];

  List<_NavItem> get _navItems => [
        _NavItem(icon: Icons.account_tree_rounded, label: 'Malla Curricular', section: 'MÓDULO ACADÉMICO'),
        _NavItem(icon: Icons.menu_book_rounded, label: 'Cursos', section: 'MÓDULO ACADÉMICO'),
        _NavItem(icon: Icons.calendar_month_rounded, label: 'Horarios', section: 'MÓDULO ACADÉMICO'),
        _NavItem(icon: Icons.fact_check_rounded, label: 'Asistencia', section: 'MÓDULO ACADÉMICO'),
        _NavItem(icon: Icons.calculate_rounded, label: 'Calculadora de Notas', section: 'MÓDULO ACADÉMICO'),
        _NavItem(icon: Icons.description_rounded, label: 'Todas las Notas', section: 'ESPACIO DE TRABAJO GENERAL'),
        _NavItem(icon: Icons.groups_rounded, label: 'Grupos Académicos', section: 'ESPACIO DE TRABAJO GENERAL'),
        _NavItem(icon: Icons.person_rounded, label: 'Perfil de Usuario', section: 'ESPACIO DE TRABAJO GENERAL'),
      ];

  void _onSelect(int index) {
    setState(() => _selectedIndex = index);
    // En móvil, cerrar drawer si está abierto
    final scaffold = Scaffold.maybeOf(context);
    if (scaffold != null && scaffold.hasDrawer && scaffold.isDrawerOpen) {
      Navigator.of(context).pop();
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;

    return Scaffold(
      backgroundColor: AppColors.bg,
      appBar: AppBar(
        backgroundColor: AppColors.surface,
        elevation: 0,
        scrolledUnderElevation: 0,
        shape: const Border(bottom: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
        leading: IconButton(
          icon: Icon(_isCollapsed ? Icons.menu_rounded : Icons.menu_open_rounded, color: AppColors.text),
          tooltip: _isCollapsed ? 'Expandir menú' : 'Colapsar menú',
          onPressed: () => setState(() => _isCollapsed = !_isCollapsed),
        ),
        title: Row(
          children: [
            Container(
              width: 36,
              height: 36,
              decoration: BoxDecoration(
                color: AppColors.accentYellow,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(6),
              ),
              child: const Center(child: Text('S', style: TextStyle(fontWeight: FontWeight.w900, color: AppColors.text))),
            ),
            const SizedBox(width: 10),
            const Text('SIGMA ACADEMY', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text, letterSpacing: -0.5)),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.logout_rounded, color: AppColors.text),
            tooltip: 'Cerrar sesión',
            onPressed: () {
              SessionManager.clear();
              Navigator.of(context).pushAndRemoveUntil(
                MaterialPageRoute(builder: (_) => const LoginScreen()),
                (r) => false,
              );
            },
          ),
          const SizedBox(width: 8),
        ],
      ),
      drawer: isDesktop ? null : _buildDrawer(),
      body: isDesktop ? _buildDesktopBody() : _buildMobileBody(),
    );
  }

  Widget _buildDesktopBody() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AnimatedContainer(
          duration: const Duration(milliseconds: 220),
          curve: Curves.easeInOut,
          width: _isCollapsed ? 72 : 280,
          decoration: const BoxDecoration(
            color: AppColors.surface,
            border: Border(right: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
          ),
          child: _buildSidebarContent(isCollapsed: _isCollapsed),
        ),
        Expanded(
          child: Container(
            color: AppColors.bg,
            child: _pages[_selectedIndex],
          ),
        ),
      ],
    );
  }

  Widget _buildMobileBody() {
    return Container(
      color: AppColors.bg,
      child: _pages[_selectedIndex],
    );
  }

  Widget _buildDrawer() {
    return Drawer(
      backgroundColor: AppColors.surface,
      shape: const RoundedRectangleBorder(
        side: BorderSide(color: AppColors.border, width: AppDimens.borderWidth),
      ),
      child: SafeArea(child: _buildSidebarContent(isCollapsed: false, isDrawer: true)),
    );
  }

  Widget _buildSidebarContent({required bool isCollapsed, bool isDrawer = false}) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 1),
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(vertical: 8),
            itemCount: _navItems.length,
            itemBuilder: (context, index) {
              final item = _navItems[index];
              final isSelected = index == _selectedIndex;
              final showSectionHeader = index == 0 || _navItems[index].section != _navItems[index - 1].section;

              return Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (showSectionHeader && !isCollapsed)
                    Padding(
                      padding: const EdgeInsets.fromLTRB(16, 16, 16, 6),
                      child: Text(item.section,
                          style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w900, color: AppColors.muted, letterSpacing: 0.8)),
                    ),
                  _SidebarTile(
                    icon: item.icon,
                    label: item.label,
                    isSelected: isSelected,
                    isCollapsed: isCollapsed,
                    onTap: () => _onSelect(index),
                  ),
                ],
              );
            },
          ),
        ),
        const Divider(color: AppColors.border, thickness: AppDimens.borderWidth, height: 1),
        Padding(
          padding: EdgeInsets.all(isCollapsed ? 8 : 12),
          child: Row(
            children: [
              Container(
                width: 36,
                height: 36,
                decoration: BoxDecoration(
                  color: AppColors.accentYellow,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(6),
                ),
                child: const Icon(Icons.person_rounded, size: 18, color: AppColors.text),
              ),
              if (!isCollapsed) ...[
                const SizedBox(width: 10),
                const Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('Estudiante', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text)),
                      Text('Sigma', style: TextStyle(fontWeight: FontWeight.w600, fontSize: 11, color: AppColors.muted)),
                    ],
                  ),
                ),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

class _NavItem {
  final IconData icon;
  final String label;
  final String section;
  const _NavItem({required this.icon, required this.label, required this.section});
}

class _SidebarTile extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isSelected;
  final bool isCollapsed;
  final VoidCallback onTap;

  const _SidebarTile({
    required this.icon,
    required this.label,
    required this.isSelected,
    required this.isCollapsed,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(6),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 180),
          padding: EdgeInsets.symmetric(horizontal: isCollapsed ? 0 : 12, vertical: 10),
          decoration: BoxDecoration(
            color: isSelected ? AppColors.accentYellow : Colors.transparent,
            border: Border.all(color: isSelected ? AppColors.border : Colors.transparent, width: 2),
            borderRadius: BorderRadius.circular(6),
            boxShadow: isSelected ? const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)] : null,
          ),
          child: Row(
            mainAxisAlignment: isCollapsed ? MainAxisAlignment.center : MainAxisAlignment.start,
            children: [
              Icon(icon, size: 20, color: AppColors.text),
              if (!isCollapsed) ...[
                const SizedBox(width: 10),
                Expanded(
                  child: Text(label,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                          fontSize: 12.5, fontWeight: isSelected ? FontWeight.w900 : FontWeight.w700, color: AppColors.text)),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}


