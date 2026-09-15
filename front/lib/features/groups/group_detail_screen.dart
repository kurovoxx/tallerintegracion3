import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../chat/group_chat_screen.dart';
import '../workspace/kanban_screen.dart';
import '../workspace/schedule_meeting_screen.dart';
import '../workspace/sprint_sheet_screen.dart';

/// Espacio de trabajo por grupo: todas las vistas colaborativas viven aquí,
/// no en la barra lateral global.
class GroupDetailScreen extends StatelessWidget {
  final Map<String, dynamic> group;

  const GroupDetailScreen({super.key, required this.group});

  @override
  Widget build(BuildContext context) {
    final groupName = group['name'] as String? ?? 'Grupo';
    final subject = group['subject'] as String? ?? 'General';
    final members = group['members'] as int? ?? 0;

    return DefaultTabController(
      length: 4,
      child: Scaffold(
        backgroundColor: AppColors.bg,
        appBar: AppBar(
          backgroundColor: AppColors.surface,
          elevation: 0,
          scrolledUnderElevation: 0,
          shape: const Border(bottom: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
          leading: IconButton(
            icon: const Icon(Icons.arrow_back_rounded, color: AppColors.text),
            onPressed: () => Navigator.of(context).pop(),
          ),
          title: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(groupName.toUpperCase(),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(fontSize: 14, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.3)),
              Text('$subject · $members integrantes',
                  style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: AppColors.muted)),
            ],
          ),
          bottom: PreferredSize(
            preferredSize: const Size.fromHeight(48),
            child: Container(
              decoration: const BoxDecoration(
                border: Border(top: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
              ),
              child: const TabBar(
                labelColor: AppColors.text,
                unselectedLabelColor: AppColors.muted,
                indicatorColor: AppColors.text,
                indicatorWeight: 3,
                labelStyle: TextStyle(fontWeight: FontWeight.w900, fontSize: 11, letterSpacing: 0.3),
                unselectedLabelStyle: TextStyle(fontWeight: FontWeight.w700, fontSize: 11),
                isScrollable: true,
                tabAlignment: TabAlignment.start,
                tabs: [
                  Tab(icon: Icon(Icons.assignment_rounded, size: 18), text: 'HOJA DE SPRINT'),
                  Tab(icon: Icon(Icons.view_kanban_rounded, size: 18), text: 'KANBAN'),
                  Tab(icon: Icon(Icons.event_rounded, size: 18), text: 'AGENDAR'),
                  Tab(icon: Icon(Icons.chat_bubble_rounded, size: 18), text: 'CHAT'),
                ],
              ),
            ),
          ),
        ),
        body: const TabBarView(
          children: [
            SprintSheetScreen(),
            KanbanScreen(),
            ScheduleMeetingScreen(),
            GroupChatScreen(),
          ],
        ),
      ),
    );
  }
}
