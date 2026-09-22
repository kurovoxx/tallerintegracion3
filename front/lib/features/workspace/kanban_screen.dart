// Ruta: front/lib/features/workspace/kanban_screen.dart
//
// 'Tablero Analógico de Taller': placas industriales, fichas técnicas con
// sellos de prioridad, etiquetas engrapadas y arrastre con feedback rígido.
// Consume tokens de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

const List<String> _kAssignees = ['Tú', 'Sofía', 'Matías', 'Ana'];

enum _TaskPriority {
  alta('Alta'),
  media('Media'),
  baja('Baja');

  const _TaskPriority(this.label);

  final String label;
}

class _KanbanTask {
  const _KanbanTask({
    required this.id,
    required this.title,
    required this.subject,
    required this.assignee,
    required this.priority,
  });

  final String id;
  final String title;
  final String subject;
  final String assignee;
  final _TaskPriority priority;
}

class _KanbanColumnData {
  _KanbanColumnData({
    required this.title,
    required this.code,
    required this.tasks,
  });

  final String title;
  final String code;
  final List<_KanbanTask> tasks;
}

class _ColumnPlateStyle {
  const _ColumnPlateStyle({
    required this.color,
    required this.foreground,
    required this.icon,
  });

  final Color color;
  final Color foreground;
  final IconData icon;
}

_ColumnPlateStyle _plateStyleFor(String title) {
  switch (title) {
    case 'En Progreso':
      return const _ColumnPlateStyle(
        color: AppColors.accentBlue,
        foreground: AppColors.surface,
        icon: Icons.engineering_rounded,
      );
    case 'Finalizado':
      return const _ColumnPlateStyle(
        color: AppColors.success,
        foreground: AppColors.text,
        icon: Icons.task_alt_rounded,
      );
    case 'Por Hacer':
    default:
      return const _ColumnPlateStyle(
        color: AppColors.accentYellow,
        foreground: AppColors.text,
        icon: Icons.construction_rounded,
      );
  }
}

class KanbanScreen extends StatefulWidget {
  const KanbanScreen({super.key});

  @override
  State<KanbanScreen> createState() => _KanbanScreenState();
}

class _KanbanScreenState extends State<KanbanScreen> {
  final PageController _pageController = PageController();
  int _selectedColumn = 0;

  final List<_KanbanColumnData> _columns = [
    _KanbanColumnData(
      title: 'Por Hacer',
      code: 'PLC-01',
      tasks: [
        const _KanbanTask(
          id: 't1',
          title: 'Investigar derivadas',
          subject: 'MAT1002',
          assignee: 'Sofía',
          priority: _TaskPriority.alta,
        ),
        const _KanbanTask(
          id: 't2',
          title: 'Resumen de apuntes',
          subject: 'MAT1002',
          assignee: 'Tú',
          priority: _TaskPriority.media,
        ),
      ],
    ),
    _KanbanColumnData(
      title: 'En Progreso',
      code: 'PLC-02',
      tasks: [
        const _KanbanTask(
          id: 't3',
          title: 'Ejercicios capítulo 3',
          subject: 'MAT1002',
          assignee: 'Matías',
          priority: _TaskPriority.alta,
        ),
      ],
    ),
    _KanbanColumnData(
      title: 'Finalizado',
      code: 'PLC-03',
      tasks: [
        const _KanbanTask(
          id: 't4',
          title: 'Mapa conceptual',
          subject: 'MAT1002',
          assignee: 'Ana',
          priority: _TaskPriority.baja,
        ),
      ],
    ),
  ];

  @override
  void dispose() {
    _pageController.dispose();
    super.dispose();
  }

  _KanbanColumnData _columnOf(_KanbanTask task) =>
      _columns.firstWhere((column) => column.tasks.contains(task));

  void _moveTask(
    _KanbanTask task,
    _KanbanColumnData from,
    _KanbanColumnData to,
  ) {
    if (identical(from, to)) return;
    setState(() {
      from.tasks.remove(task);
      to.tasks.add(task);
    });
  }

  Future<void> _openNewTaskSheet(_KanbanColumnData column) async {
    final task = await showModalBottomSheet<_KanbanTask>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.scrim,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.zero),
      builder: (_) => _NewTaskSheet(columnTitle: column.title),
    );
    if (task == null || !mounted) return;
    setState(() => column.tasks.add(task));
  }

  @override
  Widget build(BuildContext context) {
    final breakpoint = context.breakpoint;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: breakpoint == AppBreakpoint.compact
            ? _buildCompactBoard()
            : _buildWideBoard(expanded: breakpoint == AppBreakpoint.expanded),
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Layout
  // ---------------------------------------------------------------------------

  Widget _buildWideBoard({required bool expanded}) {
    return MaxWidthContainer(
      maxWidth: AppDimens.contentMaxWidth,
      padding: EdgeInsets.fromLTRB(
        expanded ? AppDimens.spaceXl : AppDimens.spaceLg,
        AppDimens.spaceXl,
        expanded ? AppDimens.spaceXl : AppDimens.spaceLg,
        AppDimens.spaceXxl,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _buildBoardHeader(compact: false),
          const SizedBox(height: AppDimens.spaceXl),
          Expanded(
            child: expanded
                ? _buildExpandedColumns()
                : _buildScrollableColumns(),
          ),
        ],
      ),
    );
  }

  Widget _buildCompactBoard() {
    return Padding(
      padding: const EdgeInsets.fromLTRB(
        AppDimens.spaceLg,
        AppDimens.spaceXl,
        AppDimens.spaceLg,
        0,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _buildBoardHeader(compact: true),
          const SizedBox(height: AppDimens.spaceLg),
          _buildCompactTabs(),
          const SizedBox(height: AppDimens.spaceLg),
          Expanded(
            child: PageView.builder(
              controller: _pageController,
              physics: const BouncingScrollPhysics(),
              onPageChanged: (index) => setState(() => _selectedColumn = index),
              itemCount: _columns.length,
              itemBuilder: (context, index) {
                final column = _columns[index];
                return Padding(
                  padding: const EdgeInsets.only(bottom: AppDimens.spaceLg),
                  child: _KanbanColumn(
                    column: column,
                    onAdd: () => _openNewTaskSheet(column),
                    onAcceptTask: (task) =>
                        _moveTask(task, _columnOf(task), column),
                    onMoveLeft: index > 0
                        ? (task) => _moveTask(task, column, _columns[index - 1])
                        : null,
                    onMoveRight: index < _columns.length - 1
                        ? (task) => _moveTask(task, column, _columns[index + 1])
                        : null,
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildExpandedColumns() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (var i = 0; i < _columns.length; i++)
          Expanded(
            child: Padding(
              padding: const EdgeInsets.symmetric(
                horizontal: AppDimens.spaceXs + 2,
              ),
              child: _KanbanColumn(
                column: _columns[i],
                onAdd: () => _openNewTaskSheet(_columns[i]),
                onAcceptTask: (task) =>
                    _moveTask(task, _columnOf(task), _columns[i]),
                onMoveLeft: i > 0
                    ? (task) => _moveTask(task, _columns[i], _columns[i - 1])
                    : null,
                onMoveRight: i < _columns.length - 1
                    ? (task) => _moveTask(task, _columns[i], _columns[i + 1])
                    : null,
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildScrollableColumns() {
    return ListView(
      scrollDirection: Axis.horizontal,
      physics: const BouncingScrollPhysics(),
      children: [
        for (var i = 0; i < _columns.length; i++)
          SizedBox(
            width: 320,
            child: Padding(
              padding: EdgeInsets.only(
                right: i == _columns.length - 1 ? 0 : AppDimens.spaceMd,
              ),
              child: _KanbanColumn(
                column: _columns[i],
                onAdd: () => _openNewTaskSheet(_columns[i]),
                onAcceptTask: (task) =>
                    _moveTask(task, _columnOf(task), _columns[i]),
                onMoveLeft: i > 0
                    ? (task) => _moveTask(task, _columns[i], _columns[i - 1])
                    : null,
                onMoveRight: i < _columns.length - 1
                    ? (task) => _moveTask(task, _columns[i], _columns[i + 1])
                    : null,
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildBoardHeader({required bool compact}) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const _PlateStamp(
                label: 'Tablero analógico de taller',
                icon: Icons.precision_manufacturing_rounded,
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                'TABLERO KANBAN',
                style: TextStyle(
                  color: AppColors.text,
                  fontWeight: FontWeight.w900,
                  fontSize: compact ? 22 : 28,
                  letterSpacing: -0.5,
                  height: 1.0,
                ),
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                'Arrastra las fichas técnicas entre placas para cambiar su estado.',
                style: TextStyle(
                  color: AppColors.mutedStrong,
                  fontWeight: FontWeight.w700,
                  fontSize: compact ? 12 : 13,
                  height: 1.3,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: AppDimens.spaceMd),
        const _SprintStamp(label: 'Sprint 3'),
      ],
    );
  }

  Widget _buildCompactTabs() {
    return Wrap(
      spacing: AppDimens.spaceSm,
      runSpacing: AppDimens.spaceSm,
      children: [
        for (var i = 0; i < _columns.length; i++)
          _BoardTabChip(
            label: '${_columns[i].title} (${_columns[i].tasks.length})',
            style: _plateStyleFor(_columns[i].title),
            active: _selectedColumn == i,
            onTap: () {
              setState(() => _selectedColumn = i);
              _pageController.animateToPage(
                i,
                duration: AppMotion.expand,
                curve: AppMotion.standard,
              );
            },
          ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Placa / columna
// ---------------------------------------------------------------------------

class _KanbanColumn extends StatelessWidget {
  const _KanbanColumn({
    required this.column,
    required this.onAdd,
    required this.onAcceptTask,
    this.onMoveLeft,
    this.onMoveRight,
  });

  final _KanbanColumnData column;
  final VoidCallback onAdd;
  final ValueChanged<_KanbanTask> onAcceptTask;
  final ValueChanged<_KanbanTask>? onMoveLeft;
  final ValueChanged<_KanbanTask>? onMoveRight;

  @override
  Widget build(BuildContext context) {
    final style = _plateStyleFor(column.title);
    return Container(
      key: ValueKey('column-${column.title}'),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: AppShadows.card,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _buildPlateHeader(style),
          Expanded(child: _buildDropZone(context)),
        ],
      ),
    );
  }

  Widget _buildPlateHeader(_ColumnPlateStyle style) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: 10,
      ),
      decoration: BoxDecoration(
        color: style.color,
        border: const Border(
          bottom: BorderSide(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
        ),
      ),
      child: Row(
        children: [
          Icon(style.icon, size: 16, color: style.foreground),
          const SizedBox(width: AppDimens.spaceSm),
          _PlateCodeTag(code: column.code, background: AppColors.surface),
          const SizedBox(width: AppDimens.spaceSm),
          Expanded(
            child: Text(
              column.title.toUpperCase(),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 12.5,
                letterSpacing: 0.8,
                color: style.foreground,
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          _PlateCountBadge(count: column.tasks.length),
          const SizedBox(width: AppDimens.spaceSm),
          NeobrutalistIconButton(
            icon: Icons.add_rounded,
            tooltip: 'Agregar ficha',
            size: 30,
            onPressed: onAdd,
          ),
        ],
      ),
    );
  }

  Widget _buildDropZone(BuildContext context) {
    return DragTarget<_KanbanTask>(
      onWillAcceptWithDetails: (_) => true,
      onAcceptWithDetails: (details) => onAcceptTask(details.data),
      builder: (context, candidate, rejected) {
        final active = candidate.isNotEmpty;
        return AnimatedContainer(
          duration: AppMotion.fast,
          curve: AppMotion.standard,
          decoration: BoxDecoration(
            color: active ? AppColors.bg : AppColors.surface,
            border: Border.all(
              color: active ? AppColors.border : Colors.transparent,
              width: AppDimens.borderWidth,
            ),
          ),
          child: Stack(
            children: [
              if (column.tasks.isEmpty)
                const Positioned.fill(child: _EmptyPlate())
              else
                ListView.builder(
                  padding: const EdgeInsets.fromLTRB(
                    AppDimens.spaceSm + 2,
                    AppDimens.spaceSm + 2,
                    AppDimens.spaceSm + 5,
                    AppDimens.spaceXs,
                  ),
                  itemCount: column.tasks.length,
                  itemBuilder: (context, index) {
                    final task = column.tasks[index];
                    return Padding(
                      key: ValueKey(task.id),
                      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
                      child: _buildDraggable(context, task),
                    );
                  },
                ),
              if (active)
                const Positioned.fill(
                  child: IgnorePointer(child: _DropHereStamp()),
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildDraggable(BuildContext context, _KanbanTask task) {
    final feedback = Transform.rotate(
      angle: 0.02,
      child: Transform.scale(
        scale: 1.02,
        child: Material(
          color: Colors.transparent,
          child: SizedBox(
            width: 270,
            child: _TaskCard(task: task, dragging: true),
          ),
        ),
      ),
    );

    final placeholder = _TaskGhost(title: task.title);

    if (context.isExpanded) {
      return Draggable<_KanbanTask>(
        data: task,
        feedback: feedback,
        childWhenDragging: placeholder,
        child: _buildCard(task),
      );
    }
    return LongPressDraggable<_KanbanTask>(
      data: task,
      feedback: feedback,
      childWhenDragging: placeholder,
      child: _buildCard(task),
    );
  }

  Widget _buildCard(_KanbanTask task) {
    return _TaskCard(
      task: task,
      onMoveLeft: onMoveLeft == null ? null : () => onMoveLeft!(task),
      onMoveRight: onMoveRight == null ? null : () => onMoveRight!(task),
    );
  }
}

// ---------------------------------------------------------------------------
// Ficha técnica (tarjeta)
// ---------------------------------------------------------------------------

class _TaskCard extends StatefulWidget {
  const _TaskCard({
    required this.task,
    this.dragging = false,
    this.onMoveLeft,
    this.onMoveRight,
  });

  final _KanbanTask task;
  final bool dragging;
  final VoidCallback? onMoveLeft;
  final VoidCallback? onMoveRight;

  @override
  State<_TaskCard> createState() => _TaskCardState();
}

class _TaskCardState extends State<_TaskCard> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final dragging = widget.dragging;
    final pressed = _pressed && !dragging;

    final List<BoxShadow> shadow;
    if (dragging) {
      shadow = AppShadows.dialog;
    } else if (pressed) {
      shadow = const <BoxShadow>[];
    } else if (_hovered) {
      shadow = AppShadows.card;
    } else {
      shadow = AppShadows.button;
    }

    return MouseRegion(
      cursor: dragging ? SystemMouseCursors.grabbing : SystemMouseCursors.grab,
      onEnter: dragging ? null : (_) => setState(() => _hovered = true),
      onExit: dragging ? null : (_) => setState(() => _hovered = false),
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTapDown: dragging ? null : (_) => setState(() => _pressed = true),
        onTapUp: dragging ? null : (_) => setState(() => _pressed = false),
        onTapCancel: dragging ? null : () => setState(() => _pressed = false),
        child: AnimatedContainer(
          duration: AppMotion.press,
          curve: AppMotion.standard,
          transform: Matrix4.translationValues(
            pressed ? AppShadows.offsetButton.dx : 0,
            pressed ? AppShadows.offsetButton.dy : 0,
            0,
          ),
          padding: const EdgeInsets.all(AppDimens.spaceMd),
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: shadow,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  _PrioritySticker(priority: widget.task.priority),
                  const Spacer(),
                  _PlateCodeTag(
                    code: widget.task.subject,
                    background: AppColors.bg,
                  ),
                  const SizedBox(width: AppDimens.spaceSm),
                  const Icon(
                    Icons.drag_indicator_rounded,
                    size: 15,
                    color: AppColors.muted,
                  ),
                ],
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                widget.task.title,
                style: const TextStyle(
                  fontWeight: FontWeight.w800,
                  fontSize: 13,
                  height: 1.25,
                  color: AppColors.text,
                ),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  Flexible(child: _AssigneeSticker(name: widget.task.assignee)),
                  const Spacer(),
                  if (widget.onMoveLeft != null) ...[
                    NeobrutalistIconButton(
                      icon: Icons.arrow_back_rounded,
                      tooltip: 'Mover a columna anterior',
                      size: 26,
                      onPressed: widget.onMoveLeft,
                    ),
                    const SizedBox(width: AppDimens.spaceXs),
                  ],
                  if (widget.onMoveRight != null)
                    NeobrutalistIconButton(
                      icon: Icons.arrow_forward_rounded,
                      tooltip: 'Mover a columna siguiente',
                      size: 26,
                      onPressed: widget.onMoveRight,
                    ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TaskGhost extends StatelessWidget {
  const _TaskGhost({required this.title});

  final String title;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      decoration: BoxDecoration(
        color: AppColors.bg,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
      ),
      child: Row(
        children: [
          const Icon(
            Icons.drag_indicator_rounded,
            size: 15,
            color: AppColors.muted,
          ),
          const SizedBox(width: AppDimens.spaceSm),
          Expanded(
            child: Text(
              'ARRASTRANDO · $title',
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 10.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
                color: AppColors.mutedStrong,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Sellos, etiquetas y badges
// ---------------------------------------------------------------------------

class _PrioritySticker extends StatelessWidget {
  const _PrioritySticker({required this.priority});

  final _TaskPriority priority;

  @override
  Widget build(BuildContext context) {
    final Color background;
    final Color foreground;
    switch (priority) {
      case _TaskPriority.alta:
        background = AppColors.error;
        foreground = AppColors.surface;
      case _TaskPriority.media:
        background = AppColors.accentYellow;
        foreground = AppColors.text;
      case _TaskPriority.baja:
        background = AppColors.surfaceLow;
        foreground = AppColors.mutedStrong;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: background,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        priority.label.toUpperCase(),
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.6,
          color: foreground,
        ),
      ),
    );
  }
}

class _AssigneeSticker extends StatelessWidget {
  const _AssigneeSticker({required this.name});

  final String name;

  @override
  Widget build(BuildContext context) {
    final initial = name.isEmpty ? '?' : name[0].toUpperCase();
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 3),
      decoration: BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 17,
            height: 17,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: AppColors.accentYellow,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(2),
            ),
            child: Text(
              initial,
              style: const TextStyle(
                fontSize: 9,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
              ),
            ),
          ),
          const SizedBox(width: 5),
          Flexible(
            child: Text(
              name,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 10.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.3,
                color: AppColors.text,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PlateCodeTag extends StatelessWidget {
  const _PlateCodeTag({required this.code, required this.background});

  final String code;
  final Color background;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
      decoration: BoxDecoration(
        color: background,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
      ),
      child: Text(
        code.toUpperCase(),
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          fontFamily: 'monospace',
          fontSize: 9.5,
          fontWeight: FontWeight.w800,
          letterSpacing: 0.4,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _PlateCountBadge extends StatelessWidget {
  const _PlateCountBadge({required this.count});

  final int count;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        '$count',
        style: const TextStyle(
          fontFamily: 'monospace',
          fontSize: 11,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _PlateStamp extends StatelessWidget {
  const _PlateStamp({required this.label, this.icon});

  final String label;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: -0.02,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 4),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radiusChip),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 12, color: AppColors.text),
              const SizedBox(width: AppDimens.spaceXs),
            ],
            Flexible(
              child: Text(
                label.toUpperCase(),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: const TextStyle(
                  fontSize: 9.5,
                  fontWeight: FontWeight.w900,
                  letterSpacing: 0.8,
                  color: AppColors.text,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SprintStamp extends StatelessWidget {
  const _SprintStamp({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: 0.02,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 6),
        decoration: BoxDecoration(
          color: AppColors.accentYellow,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radiusChip),
          boxShadow: AppShadows.badge,
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.bolt_rounded, size: 13, color: AppColors.text),
            const SizedBox(width: AppDimens.spaceXs),
            Text(
              label.toUpperCase(),
              style: const TextStyle(
                fontSize: 10,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.8,
                color: AppColors.text,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _BoardTabChip extends StatefulWidget {
  const _BoardTabChip({
    required this.label,
    required this.style,
    required this.active,
    required this.onTap,
  });

  final String label;
  final _ColumnPlateStyle style;
  final bool active;
  final VoidCallback onTap;

  @override
  State<_BoardTabChip> createState() => _BoardTabChipState();
}

class _BoardTabChipState extends State<_BoardTabChip> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final background = widget.active
        ? widget.style.color
        : (_hovered ? AppColors.surfaceLow : AppColors.surface);
    final foreground = widget.active ? widget.style.foreground : AppColors.text;

    return Semantics(
      button: true,
      selected: widget.active,
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        onEnter: (_) => setState(() => _hovered = true),
        onExit: (_) => setState(() => _hovered = false),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: (_) => setState(() => _pressed = true),
          onTapUp: (_) => setState(() => _pressed = false),
          onTapCancel: () => setState(() => _pressed = false),
          onTap: widget.onTap,
          child: AnimatedContainer(
            duration: AppMotion.press,
            curve: AppMotion.standard,
            transform: Matrix4.translationValues(
              _pressed ? AppShadows.offsetBadge.dx : 0,
              _pressed ? AppShadows.offsetBadge.dy : 0,
              0,
            ),
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
            decoration: BoxDecoration(
              color: background,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: _pressed ? const <BoxShadow>[] : AppShadows.badge,
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (widget.active) ...[
                  Icon(widget.style.icon, size: 13, color: foreground),
                  const SizedBox(width: AppDimens.spaceXs + 2),
                ],
                Text(
                  widget.label.toUpperCase(),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.6,
                    color: foreground,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _DropHereStamp extends StatelessWidget {
  const _DropHereStamp();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        padding: const EdgeInsets.symmetric(
          horizontal: AppDimens.spaceLg,
          vertical: AppDimens.spaceSm,
        ),
        decoration: BoxDecoration(
          color: AppColors.accentYellow,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radiusChip),
          boxShadow: AppShadows.badge,
        ),
        child: const Text(
          'SUELTA AQUÍ',
          style: TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w900,
            letterSpacing: 0.8,
            color: AppColors.text,
          ),
        ),
      ),
    );
  }
}

class _EmptyPlate extends StatelessWidget {
  const _EmptyPlate();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(
            Icons.layers_clear_rounded,
            size: 26,
            color: AppColors.muted,
          ),
          const SizedBox(height: AppDimens.spaceSm),
          const Text(
            'PLACA LIBRE',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: AppColors.mutedStrong,
            ),
          ),
          const SizedBox(height: AppDimens.spaceXs),
          Text(
            'Sin fichas técnicas',
            style: TextStyle(
              fontSize: 10.5,
              fontWeight: FontWeight.w700,
              color: AppColors.muted,
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Formulario modal de nueva ficha
// ---------------------------------------------------------------------------

class _NewTaskSheet extends StatefulWidget {
  const _NewTaskSheet({required this.columnTitle});

  final String columnTitle;

  @override
  State<_NewTaskSheet> createState() => _NewTaskSheetState();
}

class _NewTaskSheetState extends State<_NewTaskSheet> {
  final TextEditingController _titleController = TextEditingController();
  final TextEditingController _subjectController = TextEditingController(
    text: 'MAT1002',
  );
  _TaskPriority _priority = _TaskPriority.media;
  String _assignee = _kAssignees.first;

  @override
  void dispose() {
    _titleController.dispose();
    _subjectController.dispose();
    super.dispose();
  }

  void _submit() {
    final title = _titleController.text.trim();
    if (title.isEmpty) return;
    final subject = _subjectController.text.trim();
    Navigator.of(context).pop(
      _KanbanTask(
        id: DateTime.now().microsecondsSinceEpoch.toString(),
        title: title,
        subject: subject.isEmpty ? 'MAT1002' : subject,
        assignee: _assignee,
        priority: _priority,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      top: false,
      child: Padding(
        padding: EdgeInsets.only(
          bottom: MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Container(
          decoration: const BoxDecoration(
            color: AppColors.surface,
            border: Border(
              top: BorderSide(
                color: AppColors.border,
                width: AppDimens.borderWidthThick,
              ),
            ),
          ),
          padding: const EdgeInsets.fromLTRB(
            AppDimens.spaceXl,
            AppDimens.spaceLg,
            AppDimens.spaceXl,
            AppDimens.spaceXl,
          ),
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  children: [
                    const Flexible(
                      child: _PlateStamp(
                        label: 'Orden de trabajo',
                        icon: Icons.assignment_rounded,
                      ),
                    ),
                    const Spacer(),
                    NeobrutalistIconButton(
                      icon: Icons.close_rounded,
                      tooltip: 'Cerrar',
                      size: 32,
                      onPressed: () => Navigator.of(context).pop(),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const Text(
                  'NUEVA FICHA TÉCNICA',
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.4,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                const AppFieldLabel('Título'),
                const SizedBox(height: AppDimens.spaceSm),
                TextField(
                  controller: _titleController,
                  textInputAction: TextInputAction.next,
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    fontSize: 13.5,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration(
                    'Ej: Resolver guía de límites',
                  ),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                const AppFieldLabel('Código / curso'),
                const SizedBox(height: AppDimens.spaceSm),
                TextField(
                  controller: _subjectController,
                  textInputAction: TextInputAction.done,
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontWeight: FontWeight.w800,
                    fontSize: 13,
                    color: AppColors.text,
                  ),
                  decoration: appInputDecoration('MAT1002'),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                Row(
                  children: [
                    Expanded(
                      child: DropdownButtonFormField<_TaskPriority>(
                        initialValue: _priority,
                        isExpanded: true,
                        dropdownColor: AppColors.surface,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 12.5,
                          color: AppColors.text,
                        ),
                        decoration: appInputDecoration('Prioridad'),
                        items: [
                          for (final priority in _TaskPriority.values)
                            DropdownMenuItem(
                              value: priority,
                              child: Text(priority.label.toUpperCase()),
                            ),
                        ],
                        onChanged: (value) => setState(
                          () => _priority = value ?? _TaskPriority.media,
                        ),
                      ),
                    ),
                    const SizedBox(width: AppDimens.spaceMd),
                    Expanded(
                      child: DropdownButtonFormField<String>(
                        initialValue: _assignee,
                        isExpanded: true,
                        dropdownColor: AppColors.surface,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 12.5,
                          color: AppColors.text,
                        ),
                        decoration: appInputDecoration('Responsable'),
                        items: [
                          for (final assignee in _kAssignees)
                            DropdownMenuItem(
                              value: assignee,
                              child: Text(assignee),
                            ),
                        ],
                        onChanged: (value) => setState(
                          () => _assignee = value ?? _kAssignees.first,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceXl),
                NeobrutalistButton(
                  label: 'Agregar a ${widget.columnTitle}',
                  icon: Icons.add_task_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  expand: true,
                  onPressed: _submit,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
