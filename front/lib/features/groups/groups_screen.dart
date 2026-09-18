import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/common_widgets.dart';
import 'group_detail_screen.dart';

class GroupsScreen extends StatefulWidget {
  const GroupsScreen({super.key});

  @override
  State<GroupsScreen> createState() => _GroupsScreenState();
}

class _GroupsScreenState extends State<GroupsScreen> {
  final List<Map<String, dynamic>> _groups = [
    {
      'id': 'g1',
      'name': 'Cálculo II - Grupo Alpha',
      'subject': 'MAT1002',
      'members': 5,
      'color': AppColors.accentYellow,
      'icon': Icons.calculate_rounded,
      'avatars': ['AB', 'CD', 'EF'],
    },
    {
      'id': 'g2',
      'name': 'Bases de Datos - Proyecto',
      'subject': 'INF220',
      'members': 4,
      'color': AppColors.accentBlue,
      'icon': Icons.storage_rounded,
      'avatars': ['GH', 'IJ'],
    },
    {
      'id': 'g3',
      'name': 'Taller Integración III',
      'subject': 'INF-360',
      'members': 6,
      'color': AppColors.bg,
      'icon': Icons.handyman_rounded,
      'avatars': ['KL', 'MN', 'OP', 'QR'],
    },
  ];

  void _showCreateGroupSheet() {
    final nameCtrl = TextEditingController();
    final subjCtrl = TextEditingController();
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => Padding(
        padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
        child: Container(
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: const Border(top: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
            borderRadius: const BorderRadius.vertical(top: Radius.circular(16)),
          ),
          padding: const EdgeInsets.fromLTRB(20, 14, 20, 24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Center(child: Container(width: 44, height: 5, decoration: BoxDecoration(color: AppColors.muted, borderRadius: BorderRadius.circular(4)))),
              const SizedBox(height: 16),
              const Text('CREAR / UNIRSE A GRUPO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
              const SizedBox(height: 16),
              const AppFieldLabel('NOMBRE DEL GRUPO'),
              const SizedBox(height: 6),
              TextField(controller: nameCtrl, decoration: appInputDecoration('Ej. Grupo Cálculo')),
              const SizedBox(height: 12),
              const AppFieldLabel('ASIGNATURA ASOCIADA'),
              const SizedBox(height: 6),
              TextField(controller: subjCtrl, decoration: appInputDecoration('Ej. MAT1002')),
              const SizedBox(height: 20),
              SubmitButton(
                text: 'CREAR GRUPO',
                onPressed: () {
                  if (nameCtrl.text.trim().isEmpty) return;
                  setState(() {
                    _groups.add({
                      'id': 'g${_groups.length + 1}',
                      'name': nameCtrl.text.trim(),
                      'subject': subjCtrl.text.trim().isEmpty ? 'General' : subjCtrl.text.trim(),
                      'members': 1,
                      'color': AppColors.accentYellow,
                      'icon': Icons.groups_rounded,
                      'avatars': ['YO'],
                    });
                  });
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Grupo creado'), backgroundColor: AppColors.border));
                },
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Padding(
          padding: EdgeInsets.symmetric(horizontal: isDesktop ? 32 : 16, vertical: 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('MIS GRUPOS', style: TextStyle(fontSize: isDesktop ? 26 : 22, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.5)),
                        const SizedBox(height: 4),
                        const Text('Colabora y comparte apuntes con tu comunidad', style: TextStyle(fontSize: 12.5, fontWeight: FontWeight.w700, color: AppColors.muted)),
                      ],
                    ),
                  ),
                  const SizedBox(width: 12),
                  _InviteButton(onTap: _showCreateGroupSheet),
                ],
              ),
              const SizedBox(height: 18),
              Expanded(
                child: LayoutBuilder(builder: (context, constraints) {
                  final cols = isDesktop ? 3 : 1;
                  final w = cols == 1 ? constraints.maxWidth : (constraints.maxWidth - 14 * (cols - 1)) / cols;
                  return Wrap(
                    spacing: 14,
                    runSpacing: 14,
                    children: _groups
                        .map((g) => SizedBox(width: w, child: _GroupCard(group: g)))
                        .toList(),
                  );
                }),
              ),
            ],
          ),
        ),
      ),
      floatingActionButton: isDesktop
          ? null
          : FloatingActionButton.extended(
              backgroundColor: AppColors.border,
              foregroundColor: Colors.white,
              onPressed: _showCreateGroupSheet,
              icon: const Icon(Icons.add_rounded),
              label: const Text('NUEVO GRUPO', style: TextStyle(fontWeight: FontWeight.w900, letterSpacing: 0.5)),
            ),
    );
  }
}

class _GroupCard extends StatelessWidget {
  final Map<String, dynamic> group;
  const _GroupCard({required this.group});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: group['color'] as Color,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Icon(group['icon'] as IconData, color: AppColors.text, size: 22),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(group['name'] as String, maxLines: 1, overflow: TextOverflow.ellipsis, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 14, color: AppColors.text)),
                    const SizedBox(height: 2),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                      decoration: BoxDecoration(color: AppColors.bg, borderRadius: BorderRadius.circular(4), border: Border.all(color: AppColors.border, width: 1.5)),
                      child: Text(group['subject'] as String, style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w800, color: AppColors.text)),
                    ),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          Row(
            children: [
              SizedBox(
                height: 28,
                width: 80,
                child: Stack(
                  children: [
                    for (int i = 0; i < (group['avatars'] as List).length && i < 3; i++)
                      Positioned(
                        left: i * 18,
                        child: Container(
                          width: 28,
                          height: 28,
                          alignment: Alignment.center,
                          decoration: BoxDecoration(
                            color: AppColors.accentYellow,
                            border: Border.all(color: AppColors.border, width: 1.5),
                            shape: BoxShape.circle,
                          ),
                          child: Text((group['avatars'][i] as String), style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Text('${group['members']} integrantes', style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: AppColors.muted)),
              const Spacer(),
              const Icon(Icons.chevron_right_rounded, color: AppColors.text),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  style: OutlinedButton.styleFrom(
                    side: const BorderSide(color: AppColors.border, width: 2),
                    backgroundColor: AppColors.bg,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
                    padding: const EdgeInsets.symmetric(vertical: 10),
                  ),
                  onPressed: () => ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Invitar'))),
                  child: const Text('INVITAR', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, color: AppColors.text)),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: ElevatedButton(
                  style: ElevatedButton.styleFrom(
                    backgroundColor: AppColors.border,
                    foregroundColor: Colors.white,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
                    padding: const EdgeInsets.symmetric(vertical: 10),
                  ),
                  onPressed: () => Navigator.of(context).push(MaterialPageRoute(builder: (_) => GroupDetailScreen(group: group))),
                  child: const Text('ABRIR', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 11)),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _InviteButton extends StatelessWidget {
  final VoidCallback onTap;
  const _InviteButton({required this.onTap});

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          color: AppColors.accentYellow,
          border: Border.all(color: AppColors.border, width: 2),
          borderRadius: BorderRadius.circular(8),
          boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)],
        ),
        child: const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.add_rounded, size: 18, color: AppColors.text),
            SizedBox(width: 6),
            Text('NUEVO', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: AppColors.text)),
          ],
        ),
      ),
    );
  }
}
