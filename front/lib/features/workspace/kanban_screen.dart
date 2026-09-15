import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/common_widgets.dart';

class KanbanScreen extends StatefulWidget {
  const KanbanScreen({super.key});

  @override
  State<KanbanScreen> createState() => _KanbanScreenState();
}

class _KanbanScreenState extends State<KanbanScreen> {
  final Map<String, List<Map<String, dynamic>>> _columns = {
    'Por Hacer': [
      {'id': 't1', 'title': 'Investigar derivadas', 'assignee': 'Sofía', 'priority': 'alta'},
      {'id': 't2', 'title': 'Resumen de apuntes', 'assignee': 'Tú', 'priority': 'media'},
    ],
    'En Progreso': [
      {'id': 't3', 'title': 'Ejercicios capítulo 3', 'assignee': 'Matías', 'priority': 'alta'},
    ],
    'Finalizado': [
      {'id': 't4', 'title': 'Mapa conceptual', 'assignee': 'Ana', 'priority': 'baja'},
    ],
  };

  void _addTask(String column) {
    final ctrl = TextEditingController();
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => Padding(
        padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
        child: Container(
          decoration: BoxDecoration(color: AppColors.surface, border: const Border(top: BorderSide(color: AppColors.border, width: 2.5)), borderRadius: const BorderRadius.vertical(top: Radius.circular(16))),
          padding: const EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const Text('NUEVA TAREA', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)),
              const SizedBox(height: 12),
              TextField(controller: ctrl, decoration: appInputDecoration('Título de la tarea')),
              const SizedBox(height: 16),
              SubmitButton(
                text: 'AGREGAR A $column'.toUpperCase(),
                onPressed: () {
                  if (ctrl.text.trim().isEmpty) return;
                  setState(() => _columns[column]!.add({'id': DateTime.now().millisecondsSinceEpoch.toString(), 'title': ctrl.text.trim(), 'assignee': 'Tú', 'priority': 'media'}));
                  Navigator.pop(context);
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
          padding: EdgeInsets.symmetric(horizontal: isDesktop ? 24 : 12, vertical: 12),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Row(
                children: [
                  Expanded(child: Text('TABLERO KANBAN', style: TextStyle(fontSize: isDesktop ? 22 : 18, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: -0.5))),
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                    decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(20)),
                    child: const Text('SPRINT 3', style: TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
                  ),
                ],
              ),
              const SizedBox(height: 6),
              const Text('Arrastra tareas entre columnas para cambiar su estado', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: AppColors.muted)),
              const SizedBox(height: 16),
              Expanded(
                child: isDesktop
                    ? Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: _columns.entries
                            .map((e) => Expanded(
                                  child: Padding(
                                    padding: const EdgeInsets.symmetric(horizontal: 6),
                                    child: _KanbanColumn(
                                      title: e.key,
                                      tasks: e.value,
                                      onAdd: () => _addTask(e.key),
                                      onMove: (taskId, to) => _moveTask(taskId, e.key, to),
                                    ),
                                  ),
                                ))
                            .toList(),
                      )
                    : ListView(
                        scrollDirection: Axis.horizontal,
                        children: _columns.entries
                            .map((e) => SizedBox(
                                  width: 300,
                                  child: Padding(
                                    padding: const EdgeInsets.only(right: 12),
                                    child: _KanbanColumn(
                                      title: e.key,
                                      tasks: e.value,
                                      onAdd: () => _addTask(e.key),
                                      onMove: (taskId, to) => _moveTask(taskId, e.key, to),
                                    ),
                                  ),
                                ))
                            .toList(),
                      ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _moveTask(String taskId, String from, String to) {
    if (from == to) return;
    final task = _columns[from]!.firstWhere((t) => t['id'] == taskId);
    setState(() {
      _columns[from]!.remove(task);
      _columns[to]!.add(task);
    });
  }
}

class _KanbanColumn extends StatelessWidget {
  final String title;
  final List<Map<String, dynamic>> tasks;
  final VoidCallback onAdd;
  final Function(String taskId, String toColumn) onMove;

  const _KanbanColumn({required this.title, required this.tasks, required this.onAdd, required this.onMove});

  Color get headerColor {
    switch (title) {
      case 'Por Hacer':
        return AppColors.accentYellow;
      case 'En Progreso':
        return AppColors.accentBlue;
      case 'Finalizado':
        return const Color(0xFF22C55E);
      default:
        return AppColors.bg;
    }
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            decoration: BoxDecoration(
              color: headerColor,
              border: const Border(bottom: BorderSide(color: AppColors.border, width: AppDimens.borderWidth)),
              borderRadius: const BorderRadius.vertical(top: Radius.circular(6)),
            ),
            child: Row(
              children: [
                Text(title.toUpperCase(), style: TextStyle(fontWeight: FontWeight.w900, fontSize: 12, color: title == 'En Progreso' ? Colors.white : AppColors.text)),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(color: AppColors.surface, borderRadius: BorderRadius.circular(10), border: Border.all(color: AppColors.border, width: 1.5)),
                  child: Text('${tasks.length}', style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w900, color: AppColors.text)),
                ),
                const Spacer(),
                InkWell(onTap: onAdd, child: Container(padding: const EdgeInsets.all(4), decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: 1.5), borderRadius: BorderRadius.circular(6)), child: const Icon(Icons.add_rounded, size: 14, color: AppColors.text))),
              ],
            ),
          ),
          Expanded(
            child: DragTarget<Map<String, dynamic>>(
              onWillAcceptWithDetails: (_) => true,
              onAcceptWithDetails: (d) => onMove(d.data['id'] as String, title),
              builder: (context, cand, rej) => Container(
                color: cand.isNotEmpty ? AppColors.bg.withOpacity(0.5) : Colors.transparent,
                child: ListView.builder(
                  padding: const EdgeInsets.all(10),
                  itemCount: tasks.length,
                  itemBuilder: (context, i) {
                    final t = tasks[i];
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: Draggable<Map<String, dynamic>>(
                        data: t,
                        feedback: Material(color: Colors.transparent, child: SizedBox(width: 260, child: _TaskCard(task: t, isDragging: true))),
                        childWhenDragging: Opacity(opacity: 0.4, child: _TaskCard(task: t)),
                        child: _TaskCard(task: t),
                      ),
                    );
                  },
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _TaskCard extends StatelessWidget {
  final Map<String, dynamic> task;
  final bool isDragging;
  const _TaskCard({required this.task, this.isDragging = false});

  @override
  Widget build(BuildContext context) {
    final priority = task['priority'] as String;
    Color priColor;
    switch (priority) {
      case 'alta':
        priColor = AppColors.error;
        break;
      case 'media':
        priColor = AppColors.accentYellow;
        break;
      default:
        priColor = AppColors.muted;
    }
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 2),
        borderRadius: BorderRadius.circular(8),
        boxShadow: isDragging ? [const BoxShadow(color: AppColors.border, offset: Offset(3, 3), blurRadius: 0)] : null,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(width: 8, height: 8, decoration: BoxDecoration(color: priColor, shape: BoxShape.circle)),
              const SizedBox(width: 6),
              Text(priority.toUpperCase(), style: TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: priColor, letterSpacing: 0.5)),
              const Spacer(),
              const Icon(Icons.drag_indicator_rounded, size: 14, color: AppColors.muted),
            ],
          ),
          const SizedBox(height: 8),
          Text(task['title'] as String, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 13, color: AppColors.text)),
          const SizedBox(height: 8),
          Row(
            children: [
              Container(
                width: 22,
                height: 22,
                alignment: Alignment.center,
                decoration: BoxDecoration(color: AppColors.accentYellow, border: Border.all(color: AppColors.border, width: 1.2), shape: BoxShape.circle),
                child: Text((task['assignee'] as String)[0], style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w900, color: AppColors.text)),
              ),
              const SizedBox(width: 6),
              Text(task['assignee'] as String, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: AppColors.muted)),
            ],
          ),
        ],
      ),
    );
  }
}
