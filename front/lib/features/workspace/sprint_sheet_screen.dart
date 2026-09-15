import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';

class SprintSheetScreen extends StatefulWidget {
  const SprintSheetScreen({super.key});

  @override
  State<SprintSheetScreen> createState() => _SprintSheetScreenState();
}

class _SprintSheetScreenState extends State<SprintSheetScreen> {
  final List<String> _days = const ['Martes', 'Miércoles', 'Jueves', 'Viernes'];
  final List<String> _dayDates = const [
    '05/11/26',
    '06/11/26',
    '07/11/26',
    '08/11/26',
  ];
  final List<String> _members = const [
    'Sofía • MAT1002',
    'Matías • INF220',
    'Ana • INF-360',
    'Tú • INF-360',
  ];

  late List<Map<String, dynamic>> _tasks;

  @override
  void initState() {
    super.initState();
    _tasks = [
      {
        'member': 'Sofía • MAT1002',
        'task': 'Investigar derivadas parciales',
        'priority': 'alta',
        'status': 'En proceso',
        'est': 5.0,
        'days': {
          'Martes': 2.0,
          'Miércoles': 1.0,
          'Jueves': 0.0,
          'Viernes': 0.5,
        },
      },
      {
        'member': 'Sofía • MAT1002',
        'task': 'Resumen de apuntes SQL',
        'priority': 'media',
        'status': 'Listo',
        'est': 3.0,
        'days': {
          'Martes': 1.5,
          'Miércoles': 1.5,
          'Jueves': 0.0,
          'Viernes': 0.0,
        },
      },
      {
        'member': 'Matías • INF220',
        'task': 'Mapa OSI - presentación',
        'priority': 'alta',
        'status': 'Sin empezar',
        'est': 4.0,
        'days': {
          'Martes': 0.0,
          'Miércoles': 0.0,
          'Jueves': 2.0,
          'Viernes': 1.0,
        },
      },
      {
        'member': 'Ana • INF-360',
        'task': 'Revisión de ejercicios',
        'priority': 'baja',
        'status': 'Pendiente',
        'est': 2.0,
        'days': {
          'Martes': 0.0,
          'Miércoles': 0.0,
          'Jueves': 0.0,
          'Viernes': 1.0,
        },
      },
      {
        'member': 'Tú • INF-360',
        'task': 'Setup Drift FTS5',
        'priority': 'alta',
        'status': 'En proceso',
        'est': 6.0,
        'days': {
          'Martes': 2.0,
          'Miércoles': 2.0,
          'Jueves': 1.0,
          'Viernes': 0.0,
        },
      },
    ];
  }

  double _used(Map<String, dynamic> t) =>
      (t['days'] as Map<String, double>).values.fold(0, (a, b) => a + b);
  double _remaining(Map<String, dynamic> t) =>
      ((t['est'] as double) - _used(t)).clamp(0, 999).toDouble();

  double get totalEst => _tasks.fold(0, (s, t) => s + (t['est'] as double));
  double get totalUsed => _tasks.fold(0, (s, t) => s + _used(t));
  double get totalRemaining => (totalEst - totalUsed).clamp(0, 999).toDouble();
  int get totalDays => _days.length;

  Color _statusColor(String s) {
    switch (s) {
      case 'En proceso':
        return const Color(0xFFFFD700);
      case 'Sin empezar':
        return const Color(0xFFB5FF00);
      case 'Listo':
        return const Color(0xFF00FF00);
      case 'Pendiente':
        return const Color(0xFFFF5500);
      default:
        return AppColors.bg;
    }
  }

  void _addTaskForMember(String member) {
    final ctrl = TextEditingController();
    showDialog(
      context: context,
      builder: (_) => AlertDialog(
        backgroundColor: AppColors.surface,
        shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.zero,
          side: BorderSide(color: Colors.black, width: 2),
        ),
        title: Text(
          'NUEVA TAREA • $member',
          style: const TextStyle(
            fontWeight: FontWeight.w900,
            fontSize: 12,
            color: AppColors.text,
          ),
        ),
        content: TextField(
          controller: ctrl,
          decoration: const InputDecoration(
            hintText: 'Título',
            filled: true,
            fillColor: AppColors.bg,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.zero,
              borderSide: BorderSide(color: Colors.black, width: 2),
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text(
              'CANCELAR',
              style: TextStyle(
                color: AppColors.text,
                fontWeight: FontWeight.w800,
              ),
            ),
          ),
          ElevatedButton(
            style: ElevatedButton.styleFrom(
              backgroundColor: Colors.black,
              foregroundColor: Colors.white,
              shape: const RoundedRectangleBorder(
                borderRadius: BorderRadius.zero,
              ),
              side: const BorderSide(color: Colors.black, width: 2),
            ),
            onPressed: () {
              if (ctrl.text.trim().isEmpty) return;
              setState(() {
                _tasks.add({
                  'member': member,
                  'task': ctrl.text.trim(),
                  'priority': 'media',
                  'status': 'Sin empezar',
                  'est': 2.0,
                  'days': {for (var d in _days) d: 0.0},
                });
              });
              Navigator.pop(context);
            },
            child: const Text(
              'AGREGAR',
              style: TextStyle(fontWeight: FontWeight.w900),
            ),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SingleChildScrollView(
        scrollDirection: Axis.vertical,
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              'HOJA DE SPRINT',
              style: TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
                letterSpacing: -0.5,
                shadows: [
                  Shadow(
                    offset: Offset(1, 1),
                    color: Colors.black.withValues(alpha: 0.0),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            _buildMetricsHeader(),
            const SizedBox(height: 12),
            _buildLegendBar(),
            const SizedBox(height: 16),
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: SizedBox(width: 1200, child: _buildSprintTable()),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildMetricsHeader() {
    final now = DateTime.now();
    final end = now.add(const Duration(days: 7));
    String fmt(DateTime d) =>
        '${d.day.toString().padLeft(2, '0')}/${d.month.toString().padLeft(2, '0')}/${d.year.toString().substring(2)}';

    return Container(
      height: 75,
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: Colors.black, width: 2),
        boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4))],
      ),
      child: Row(
        children: [
          Expanded(
            flex: 2,
            child: Container(
              color: const Color(0xFFF3F3F4),
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'FECHAS DEL SPRINT',
                    style: TextStyle(
                      fontSize: 9,
                      fontWeight: FontWeight.w800,
                      color: AppColors.muted,
                      letterSpacing: 0.5,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Row(
                    children: [
                      Expanded(
                        child: _DateCell(label: 'Actual', date: fmt(now)),
                      ),
                      const SizedBox(width: 6),
                      const Icon(
                        Icons.arrow_right_alt_rounded,
                        size: 14,
                        color: AppColors.text,
                      ),
                      const SizedBox(width: 6),
                      Expanded(
                        child: _DateCell(label: 'Fin', date: fmt(end)),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
          const VerticalDivider(width: 2, thickness: 2, color: Colors.black),
          Expanded(
            child: _buildMetricCell(
              'TOTAL HORAS',
              totalEst.toStringAsFixed(1),
              const Color(0xFFE6F4EA),
            ),
          ),
          const VerticalDivider(width: 2, thickness: 2, color: Colors.black),
          Expanded(
            child: _buildMetricCell(
              'DÍAS TOTALES',
              '$totalDays',
              const Color(0xFFE6F4EA),
            ),
          ),
          const VerticalDivider(width: 2, thickness: 2, color: Colors.black),
          Expanded(
            child: _buildMetricCell(
              'HORAS USADAS',
              totalUsed.toStringAsFixed(1),
              const Color(0xFFE6F4EA),
            ),
          ),
          const VerticalDivider(width: 2, thickness: 2, color: Colors.black),
          Expanded(
            child: _buildMetricCell(
              'HORAS RESTANTES',
              totalRemaining.toStringAsFixed(1),
              const Color(0xFFFFD700),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildMetricCell(String label, String value, Color bg) {
    return Container(
      color: bg,
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 10),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Text(
            label,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w800,
              color: AppColors.muted,
              letterSpacing: 0.4,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            value,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildLegendBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: Colors.black, width: 2),
        boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4))],
      ),
      child: Wrap(
        spacing: 10,
        runSpacing: 6,
        children: const [
          _LegendChip(color: Color(0xFFFFD700), label: 'En proceso'),
          _LegendChip(color: Color(0xFFB5FF00), label: 'Sin empezar'),
          _LegendChip(color: Color(0xFF00FF00), label: 'Listo'),
          _LegendChip(color: Color(0xFFFF5500), label: 'Pendiente'),
        ],
      ),
    );
  }

  Widget _buildSprintTable() {
    final grouped = <String, List<Map<String, dynamic>>>{};
    for (final m in _members) {
      grouped[m] = _tasks.where((t) => t['member'] == m).toList();
    }

    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        border: Border.all(color: Colors.black, width: 2),
        boxShadow: const [BoxShadow(color: Colors.black, offset: Offset(4, 4))],
      ),
      child: Table(
        border: TableBorder.all(color: Colors.black, width: 2),
        columnWidths: const {
          0: FixedColumnWidth(220),
          1: FixedColumnWidth(90),
          2: FixedColumnWidth(110),
          3: FixedColumnWidth(110),
          4: FixedColumnWidth(100),
          5: FixedColumnWidth(110),
          6: FixedColumnWidth(90),
          7: FixedColumnWidth(90),
          8: FixedColumnWidth(90),
          9: FixedColumnWidth(90),
        },
        children: [
          // Header fila 1: Tareas/Prioridad/Estado amarillo, Horas blanco, Fechas gris
          TableRow(
            children: [
              _HeaderCellFixed('TAREAS', 220, bg: const Color(0xFFFFD700)),
              _HeaderCellFixed('PRIORIDAD', 90, bg: const Color(0xFFFFD700)),
              _HeaderCellFixed('ESTADO', 110, bg: const Color(0xFFFFD700)),
              _HeaderCellFixed('HORAS ASIG.', 110, bg: Colors.white),
              _HeaderCellFixed('HORAS USADAS', 100, bg: Colors.white),
              _HeaderCellFixed('HORAS REST.', 110, bg: Colors.white),
              for (int i = 0; i < _days.length; i++)
                Container(
                  height: 28,
                  color: const Color(0xFFE2E8F0),
                  alignment: Alignment.center,
                  child: Text(
                    _dayDates[i],
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text,
                    ),
                  ),
                ),
            ],
          ),
          // Header fila 2: días en negro
          TableRow(
            children: [
              Container(height: 26, color: const Color(0xFFFFD700)),
              Container(height: 26, color: const Color(0xFFFFD700)),
              Container(height: 26, color: const Color(0xFFFFD700)),
              Container(height: 26, color: Colors.white),
              Container(height: 26, color: Colors.white),
              Container(height: 26, color: Colors.white),
              for (final d in _days)
                Container(
                  height: 26,
                  color: Colors.black,
                  alignment: Alignment.center,
                  child: Text(
                    d.toUpperCase(),
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: Colors.white,
                    ),
                  ),
                ),
            ],
          ),
          // Filas por integrante
          for (final member in _members) ...[
            TableRow(
              decoration: const BoxDecoration(color: Color(0xFFF3F3F4)),
              children: [
                Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 6,
                  ),
                  child: Row(
                    children: [
                      Expanded(
                        child: Text(
                          member,
                          style: const TextStyle(
                            fontWeight: FontWeight.w900,
                            fontSize: 11,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                      InkWell(
                        onTap: () => _addTaskForMember(member),
                        child: Container(
                          width: 22,
                          height: 22,
                          alignment: Alignment.center,
                          decoration: BoxDecoration(
                            color: const Color(0xFFFFD700),
                            border: Border.all(color: Colors.black, width: 2),
                          ),
                          child: const Icon(
                            Icons.add_rounded,
                            size: 14,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                const SizedBox(height: 32),
                for (final _ in _days) const SizedBox(height: 32),
              ],
            ),
            for (final t in grouped[member]!)
              TableRow(
                children: [
                  Padding(
                    padding: const EdgeInsets.all(6),
                    child: Text(
                      t['task'] as String,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 10,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                  Center(
                    child: _PriorityChip(priority: t['priority'] as String),
                  ),
                  Center(
                    child: _StatusChip(
                      status: t['status'] as String,
                      color: _statusColor(t['status'] as String),
                    ),
                  ),
                  Center(
                    child: Text(
                      '${(t['est'] as double).toStringAsFixed(1)}h',
                      style: const TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                      ),
                    ),
                  ),
                  Center(
                    child: Text(
                      '${_used(t).toStringAsFixed(1)}h',
                      style: const TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                      ),
                    ),
                  ),
                  Center(
                    child: Text(
                      '${_remaining(t).toStringAsFixed(1)}h',
                      style: TextStyle(
                        fontWeight: FontWeight.w800,
                        fontSize: 10,
                        color: _remaining(t) == 0
                            ? Color(0xFF22C55E)
                            : AppColors.text,
                      ),
                    ),
                  ),
                  for (final d in _days)
                    Container(
                      height: 32,
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        color:
                            (((t['days'] as Map<String, double>)[d] ?? 0) > 0)
                            ? const Color(0xFFFFD700)
                            : Colors.white,
                        border: Border.all(color: Colors.black, width: 1.5),
                      ),
                      child: Text(
                        '${((t['days'] as Map<String, double>)[d] ?? 0).toStringAsFixed(1)}h',
                        style: const TextStyle(
                          fontWeight: FontWeight.w800,
                          fontSize: 10,
                          color: AppColors.text,
                        ),
                      ),
                    ),
                ],
              ),
          ],
        ],
      ),
    );
  }
}

class _DateCell extends StatelessWidget {
  final String label;
  final String date;
  const _DateCell({required this.label, required this.date});

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 9,
            fontWeight: FontWeight.w800,
            color: AppColors.muted,
          ),
        ),
        const SizedBox(height: 2),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
          decoration: BoxDecoration(
            color: AppColors.bg,
            border: Border.all(color: Colors.black, width: 1),
          ),
          child: Text(
            date,
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ),
      ],
    );
  }
}

class _HeaderCellFixed extends StatelessWidget {
  final String text;
  final double width;
  final Color bg;
  const _HeaderCellFixed(this.text, this.width, {this.bg = Colors.white});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: width,
      height: 28,
      alignment: Alignment.center,
      decoration: BoxDecoration(color: bg, border: Border.all(color: Colors.black, width: 1.5)),
      child: Text(text, textAlign: TextAlign.center, style: const TextStyle(fontWeight: FontWeight.w900, fontSize: 9, color: AppColors.text)),
    );
  }
}

class _LegendChip extends StatelessWidget {
  final Color color;
  final String label;
  const _LegendChip({required this.color, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(color: Colors.black, width: 1.5),
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
  }
}

class _PriorityChip extends StatelessWidget {
  final String priority;
  const _PriorityChip({required this.priority});

  @override
  Widget build(BuildContext context) {
    Color bg;
    Color fg;
    switch (priority) {
      case 'alta':
        bg = const Color(0xFFFF5500);
        fg = Colors.white;
        break;
      case 'media':
        bg = const Color(0xFFFFD700);
        fg = AppColors.text;
        break;
      default:
        bg = const Color(0xFFE2E8F0);
        fg = AppColors.text;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
      decoration: BoxDecoration(
        color: bg,
        border: Border.all(color: Colors.black, width: 1.5),
      ),
      child: Text(
        priority.toUpperCase(),
        style: TextStyle(fontSize: 9, fontWeight: FontWeight.w900, color: fg),
        textAlign: TextAlign.center,
      ),
    );
  }
}

class _StatusChip extends StatelessWidget {
  final String status;
  final Color color;
  const _StatusChip({required this.status, required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(color: Colors.black, width: 1.5),
      ),
      child: Text(
        status.toUpperCase(),
        textAlign: TextAlign.center,
        style: const TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
  }
}
