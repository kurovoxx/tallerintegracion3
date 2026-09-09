

import 'package:flutter/material.dart';
import '../../core/common_widgets.dart';
import '../../core/theme/app_theme.dart';

enum _ViewMode { byDay, fullWeek }

const List<String> _kWeekDayLabels = ['DOM', 'LUN', 'MAR', 'MIÉ', 'JUE', 'VIE', 'SÁB'];
const List<int> _kBaseWeekdays = [1, 2, 3, 4, 5];
const int _kStartHour = 8;
const int _kEndHour = 19;
const double _kHourHeight = 64;
const double _kWeekDayColumnWidth = 138;
const double _kHourLabelColumnWidth = 50;
const double _kMinZoom = 0.55;
const double _kMaxZoom = 1.80;
const double _kZoomStep = 0.15;

const List<Color> _kSubjectPalette = [
  AppColors.accentYellow,
  Color(0xFFD6E3FF),
  Color(0xFFFFDAD6),
  Colors.white,
];

int _timeToMinutes(String time) {
  final parts = time.split(':');
  return int.parse(parts[0]) * 60 + int.parse(parts[1]);
}

String _formatTime(String time) {
  final parts = time.split(':');
  return '${parts[0].padLeft(2, '0')}:${parts[1].padLeft(2, '0')}';
}

class ScheduleScreen extends StatefulWidget {
  const ScheduleScreen({super.key});

  @override
  State<ScheduleScreen> createState() => _ScheduleScreenState();
}

class _ScheduleScreenState extends State<ScheduleScreen> {
  bool _isLoading = true;
  bool _hasError = false;
  List<Map<String, dynamic>> _schedules = [];
  int _selectedDay = 1;
  _ViewMode _viewMode = _ViewMode.fullWeek;
  double _weekZoom = 1.0;
  double _zoomAtScaleStart = 1.0;

  @override
  void initState() {
    super.initState();
    _loadSchedule();
  }

  Future<void> _loadSchedule() async {
    if (_isLoading && _schedules.isNotEmpty) return;

    setState(() {
      _isLoading = true;
      _hasError = false;
    });

    try {
      await Future.delayed(const Duration(milliseconds: 500));
      _schedules = [
        {'id': 'sch-1', 'subject_id': 'calc-3', 'subject_name': 'Cálculo III', 'day_of_week': 1, 'start_time': '10:00', 'end_time': '11:30', 'room': 'CJP11-204', 'is_customized': false},
        {'id': 'sch-2', 'subject_id': 'calc-3', 'subject_name': 'Cálculo III', 'day_of_week': 3, 'start_time': '10:00', 'end_time': '11:30', 'room': 'CJP11-204', 'is_customized': false},
        {'id': 'sch-3', 'subject_id': 'taller-3', 'subject_name': 'Taller de Integración III', 'day_of_week': 2, 'start_time': '08:30', 'end_time': '10:00', 'room': 'CJP11-102', 'is_customized': false},
        {'id': 'sch-4', 'subject_id': 'taller-3', 'subject_name': 'Prog. Estructurada', 'day_of_week': 4, 'start_time': '08:30', 'end_time': '10:00', 'room': 'CJP11-102', 'is_customized': false},
        {'id': 'sch-5', 'subject_id': 'redes', 'subject_name': 'Redes de Computadores', 'day_of_week': 4, 'start_time': '11:30', 'end_time': '13:00', 'room': 'CJP11-101', 'is_customized': false},
        {'id': 'sch-5b', 'subject_id': 'ayudantia-redes', 'subject_name': 'Ayudantía Redes', 'day_of_week': 4, 'start_time': '12:00', 'end_time': '13:00', 'room': 'Lab-3', 'is_customized': true},
        {'id': 'sch-6', 'subject_id': 'seginf', 'subject_name': 'Seguridad Informática', 'day_of_week': 5, 'start_time': '14:00', 'end_time': '17:00', 'room': 'CJP11-102', 'is_customized': true},
        {'id': 'sch-7', 'subject_id': 'lab-redes-sab', 'subject_name': 'Laboratorio de Redes', 'day_of_week': 6, 'start_time': '09:00', 'end_time': '11:00', 'room': 'Lab-Redes', 'is_customized': true},
      ];

      final todayPostgres = DateTime.now().weekday % 7;
      final displayed = _displayedDays;
      _selectedDay = displayed.contains(todayPostgres) ? todayPostgres : displayed.first;

      if (!mounted) return;
      setState(() => _isLoading = false);
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
      });
    }
  }

  List<int> get _displayedDays {
    final extra = _schedules.map((s) => s['day_of_week'] as int).where((d) => d == 0 || d == 6).toSet();
    final days = {..._kBaseWeekdays, ...extra}.toList()..sort();
    return days;
  }

  Color _colorForSubject(String subjectId) {
    final index = subjectId.hashCode.abs() % _kSubjectPalette.length;
    return _kSubjectPalette[index];
  }

  List<Map<String, dynamic>> _schedulesForDay(int day) {
    final list = _schedules.where((s) => s['day_of_week'] == day).toList();
    list.sort((a, b) => _timeToMinutes(a['start_time'] as String).compareTo(_timeToMinutes(b['start_time'] as String)));
    return list;
  }

  void _changeZoom(double delta) {
    setState(() => _weekZoom = (_weekZoom + delta).clamp(_kMinZoom, _kMaxZoom));
  }

  void _fitWeekToWidth(double availableWidth) {
    final days = _displayedDays.length;
    if (days == 0 || availableWidth <= 0) return;
    const gap = 8.0;
    final naturalWidth = days * _kWeekDayColumnWidth + (days - 1) * gap;
    setState(() => _weekZoom = (availableWidth / naturalWidth).clamp(_kMinZoom, _kMaxZoom));
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;

    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: _hasError
            ? _buildErrorState()
            : SingleChildScrollView(
                padding: EdgeInsets.symmetric(horizontal: isDesktop ? 32 : 18, vertical: 22),
                child: isDesktop
                    ? Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          _buildHeader(),
                          const SizedBox(height: 18),
                          _buildViewModeSelector(),
                          const SizedBox(height: 18),
                          if (_isLoading)
                            _buildLoadingCard()
                          else if (_viewMode == _ViewMode.fullWeek)
                            _buildWeekView()
                          else
                            _buildDayView(),
                        ],
                      )
                    : Center(
                        child: ConstrainedBox(
                          constraints: const BoxConstraints(maxWidth: 640),
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              _buildHeader(),
                              const SizedBox(height: 18),
                              _buildViewModeSelector(),
                              const SizedBox(height: 18),
                              if (_isLoading)
                                _buildLoadingCard()
                              else if (_viewMode == _ViewMode.fullWeek)
                                _buildWeekView()
                              else
                                _buildDayView(),
                            ],
                          ),
                        ),
                      ),
              ),
      ),
    );
  }

  Widget _buildHeader() {
    final canPop = Navigator.of(context).canPop();
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        if (canPop) ...[
          _SquareIconButton(icon: Icons.arrow_back_rounded, tooltip: 'Volver', onPressed: () => Navigator.of(context).pop()),
          const SizedBox(width: 12),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: const [
              Text('MI HORARIO', style: TextStyle(color: AppColors.text, fontWeight: FontWeight.w900, fontSize: 28, letterSpacing: -0.6)),
              SizedBox(height: 2),
              Text('Bloques de clases de tu semestre actual', style: TextStyle(color: AppColors.muted, fontWeight: FontWeight.w700, fontSize: 13.5)),
            ],
          ),
        ),
        _SquareIconButton(icon: Icons.refresh_rounded, tooltip: 'Actualizar', onPressed: _loadSchedule),
      ],
    );
  }

  Widget _buildViewModeSelector() {
    return Row(
      children: [
        Expanded(child: _SegmentedDayButton(label: 'POR DÍA', active: _viewMode == _ViewMode.byDay, onTap: () => setState(() => _viewMode = _ViewMode.byDay))),
        const SizedBox(width: 10),
        Expanded(child: _SegmentedDayButton(label: 'SEMANA COMPLETA', active: _viewMode == _ViewMode.fullWeek, onTap: () => setState(() => _viewMode = _ViewMode.fullWeek))),
      ],
    );
  }

  Widget _buildDayView() {
    final gridHeight = (_kEndHour - _kStartHour) * _kHourHeight;
    final daySchedules = _schedulesForDay(_selectedDay);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _buildDaySelector(),
        const SizedBox(height: 14),
        Container(
          padding: const EdgeInsets.all(14),
          decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
          child: daySchedules.isEmpty
              ? _buildEmptyDay()
              : SizedBox(
                  height: gridHeight,
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      SizedBox(width: 48, child: _HourLabelsColumn(gridHeight: gridHeight, hourHeight: _kHourHeight)),
                      const SizedBox(width: 8),
                      Expanded(child: _DayColumn(gridHeight: gridHeight, hourHeight: _kHourHeight, schedules: daySchedules, colorResolver: _colorForSubject)),
                    ],
                  ),
                ),
        ),
      ],
    );
  }

  Widget _buildDaySelector() {
    final days = _displayedDays;
    return Row(
      children: [
        for (int i = 0; i < days.length; i++) ...[
          if (i != 0) const SizedBox(width: 6),
          Expanded(child: _SegmentedDayButton(label: _kWeekDayLabels[days[i]], active: _selectedDay == days[i], onTap: () => setState(() => _selectedDay = days[i]))),
        ],
      ],
    );
  }

  Widget _buildEmptyDay() {
    return SizedBox(height: 160, child: Center(child: Column(mainAxisSize: MainAxisSize.min, children: const [Icon(Icons.event_available_rounded, size: 36, color: AppColors.muted), SizedBox(height: 10), Text('SIN CLASES PROGRAMADAS', style: TextStyle(fontWeight: FontWeight.w900, fontSize: 13, color: AppColors.muted))])));
  }

  Widget _buildWeekView() {
    final days = _displayedDays;
    final totalHours = _kEndHour - _kStartHour;
    final gridHeight = totalHours * _kHourHeight;
    final isDesktop = MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
      child: LayoutBuilder(
        builder: (context, constraints) {
          // Desktop: ocupar todo el ancho con Expanded por columna
          if (isDesktop) {
            return Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _buildZoomToolbar(constraints.maxWidth - _kHourLabelColumnWidth - 8),
                const SizedBox(height: 12),
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    SizedBox(
                      width: _kHourLabelColumnWidth,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const SizedBox(height: 40),
                          _HourLabelsColumn(gridHeight: gridHeight, hourHeight: _kHourHeight),
                        ],
                      ),
                    ),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          for (int i = 0; i < days.length; i++) ...[
                            if (i != 0) const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  _DayHeaderChip(label: _kWeekDayLabels[days[i]]),
                                  const SizedBox(height: 10),
                                  _DayColumn(gridHeight: gridHeight, hourHeight: _kHourHeight, schedules: _schedulesForDay(days[i]), colorResolver: _colorForSubject),
                                ],
                              ),
                            ),
                          ],
                        ],
                      ),
                    ),
                  ],
                ),
              ],
            );
          }

          // Mobile / Tablet: scroll horizontal con zoom
          final dayViewportWidth = (constraints.maxWidth - _kHourLabelColumnWidth - 8).clamp(0.0, double.infinity);
          final scaledDayWidth = _kWeekDayColumnWidth * _weekZoom;
          final scaledHourHeight = _kHourHeight * _weekZoom;
          final scaledGridHeight = totalHours * scaledHourHeight;

          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              _buildZoomToolbar(dayViewportWidth),
              const SizedBox(height: 12),
              Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  SizedBox(
                    width: _kHourLabelColumnWidth,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const SizedBox(height: 40),
                        _HourLabelsColumn(gridHeight: scaledGridHeight, hourHeight: scaledHourHeight),
                      ],
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: GestureDetector(
                      onScaleStart: (_) => _zoomAtScaleStart = _weekZoom,
                      onScaleUpdate: (details) {
                        if (details.scale != 1.0) {
                          setState(() => _weekZoom = (_zoomAtScaleStart * details.scale).clamp(_kMinZoom, _kMaxZoom));
                        }
                      },
                      child: SingleChildScrollView(
                        scrollDirection: Axis.horizontal,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                for (int i = 0; i < days.length; i++) ...[
                                  if (i != 0) SizedBox(width: 8 * _weekZoom),
                                  SizedBox(width: scaledDayWidth, child: _DayHeaderChip(label: _kWeekDayLabels[days[i]], fontSize: (13 * _weekZoom).clamp(9.0, 15.0), verticalPadding: (10 * _weekZoom).clamp(6.0, 14.0))),
                                ],
                              ],
                            ),
                            SizedBox(height: 10 * _weekZoom),
                            Row(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                for (int i = 0; i < days.length; i++) ...[
                                  if (i != 0) SizedBox(width: 8 * _weekZoom),
                                  SizedBox(
                                    width: scaledDayWidth,
                                    child: _DayColumn(gridHeight: scaledGridHeight, hourHeight: scaledHourHeight, schedules: _schedulesForDay(days[i]), colorResolver: _colorForSubject, zoom: _weekZoom),
                                  ),
                                ],
                              ],
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ],
          );
        },
      ),
    );
  }

  Widget _buildZoomToolbar(double dayViewportWidth) {
    final percentage = (_weekZoom * 100).round();
    return Row(
      children: [
        const Icon(Icons.zoom_in_rounded, size: 18, color: AppColors.text),
        const SizedBox(width: 7),
        const Text('ZOOM HORARIO', style: TextStyle(fontSize: 11, fontWeight: FontWeight.w900, color: AppColors.text, letterSpacing: 0.3)),
        const Spacer(),
        _ZoomButton(icon: Icons.remove_rounded, tooltip: 'Alejar', enabled: _weekZoom > _kMinZoom, onPressed: () => _changeZoom(-_kZoomStep)),
        SizedBox(width: 54, child: Text('$percentage%', textAlign: TextAlign.center, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w900, color: AppColors.text))),
        _ZoomButton(icon: Icons.add_rounded, tooltip: 'Acercar', enabled: _weekZoom < _kMaxZoom, onPressed: () => _changeZoom(_kZoomStep)),
        const SizedBox(width: 8),
        _ZoomButton(icon: Icons.fit_screen_rounded, tooltip: 'Ajustar semana a pantalla', enabled: true, onPressed: () => _fitWeekToWidth(dayViewportWidth)),
      ],
    );
  }

  Widget _buildLoadingCard() {
    return Container(
      height: 260,
      decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(4, 4), blurRadius: 0)]),
      child: const Center(child: CircularProgressIndicator(color: AppColors.text)),
    );
  }

  Widget _buildErrorState() {
    return Center(child: Padding(padding: const EdgeInsets.all(24), child: Column(mainAxisSize: MainAxisSize.min, children: [const Icon(Icons.error_outline_rounded, size: 48, color: AppColors.text), const SizedBox(height: 12), const Text('NO SE PUDO CARGAR TU HORARIO', textAlign: TextAlign.center, style: TextStyle(fontWeight: FontWeight.w900, fontSize: 16, color: AppColors.text)), const SizedBox(height: 16), SizedBox(width: 200, child: SubmitButton(text: 'REINTENTAR', onPressed: _loadSchedule))])));
  }
}

class _DayHeaderChip extends StatelessWidget {
  final String label;
  final double fontSize;
  final double verticalPadding;
  const _DayHeaderChip({required this.label, this.fontSize = 13, this.verticalPadding = 10});

  @override
  Widget build(BuildContext context) {
    return Container(padding: EdgeInsets.symmetric(vertical: verticalPadding), alignment: Alignment.center, decoration: BoxDecoration(color: AppColors.bg, border: Border.all(color: AppColors.border, width: 2), borderRadius: BorderRadius.circular(AppDimens.radius)), child: Text(label, style: TextStyle(fontWeight: FontWeight.w900, fontSize: fontSize, letterSpacing: 0.5, color: AppColors.text)));
  }
}

class _HourLabelsColumn extends StatelessWidget {
  final double gridHeight;
  final double hourHeight;
  const _HourLabelsColumn({required this.gridHeight, required this.hourHeight});

  @override
  Widget build(BuildContext context) {
    final hours = List.generate(_kEndHour - _kStartHour, (i) => _kStartHour + i);
    final fontSize = (11 * (hourHeight / _kHourHeight)).clamp(8.0, 14.0);
    return SizedBox(height: gridHeight, width: double.infinity, child: Stack(children: [for (final hour in hours) Positioned(top: (hour - _kStartHour) * hourHeight, left: 0, right: 0, child: Text('${hour.toString().padLeft(2, '0')}:00', textAlign: TextAlign.right, style: TextStyle(fontSize: fontSize, fontWeight: FontWeight.w700, color: AppColors.muted)))]));
  }
}

class _PositionedSchedule {
  final Map<String, dynamic> schedule;
  final int lane;
  final int laneCount;
  _PositionedSchedule({required this.schedule, required this.lane, required this.laneCount});
}

class _DayColumn extends StatelessWidget {
  final double gridHeight;
  final double hourHeight;
  final List<Map<String, dynamic>> schedules;
  final Color Function(String subjectId) colorResolver;
  final double zoom;

  const _DayColumn({required this.gridHeight, required this.hourHeight, required this.schedules, required this.colorResolver, this.zoom = 1.0});

  List<_PositionedSchedule> _resolveOverlaps() {
    final sorted = [...schedules]..sort((a, b) => _timeToMinutes(a['start_time'] as String).compareTo(_timeToMinutes(b['start_time'] as String)));
    final result = <_PositionedSchedule>[];
    List<Map<String, dynamic>> cluster = [];
    int clusterEnd = -1;

    void flushCluster() {
      if (cluster.isEmpty) return;
      final laneEndTimes = <int>[];
      final laneOf = <Map<String, dynamic>, int>{};
      for (final s in cluster) {
        final start = _timeToMinutes(s['start_time'] as String);
        final end = _timeToMinutes(s['end_time'] as String);
        int? freeLane;
        for (int i = 0; i < laneEndTimes.length; i++) {
          if (laneEndTimes[i] <= start) {
            freeLane = i;
            break;
          }
        }
        if (freeLane == null) {
          freeLane = laneEndTimes.length;
          laneEndTimes.add(end);
        } else {
          laneEndTimes[freeLane] = end;
        }
        laneOf[s] = freeLane;
      }
      final laneCount = laneEndTimes.length;
      for (final s in cluster) result.add(_PositionedSchedule(schedule: s, lane: laneOf[s]!, laneCount: laneCount));
      cluster = [];
    }

    for (final s in sorted) {
      final start = _timeToMinutes(s['start_time'] as String);
      final end = _timeToMinutes(s['end_time'] as String);
      if (cluster.isEmpty || start < clusterEnd) {
        cluster.add(s);
        clusterEnd = clusterEnd == -1 ? end : (end > clusterEnd ? end : clusterEnd);
      } else {
        flushCluster();
        cluster.add(s);
        clusterEnd = end;
      }
    }
    flushCluster();
    return result;
  }

  @override
  Widget build(BuildContext context) {
    final totalHours = _kEndHour - _kStartHour;
    final positioned = _resolveOverlaps();

    return LayoutBuilder(builder: (context, constraints) {
      final columnWidth = constraints.maxWidth.isFinite ? constraints.maxWidth : _kWeekDayColumnWidth;
      return Container(
        height: gridHeight,
        width: columnWidth,
        clipBehavior: Clip.hardEdge,
        decoration: BoxDecoration(border: Border.all(color: AppColors.muted.withOpacity(0.35), width: 1)),
        child: Stack(children: [
          for (int i = 0; i <= totalHours; i++) Positioned(top: i * hourHeight, left: 0, right: 0, child: Container(height: 1, color: AppColors.muted.withOpacity(0.2))),
          for (final p in positioned) _buildBlock(p, columnWidth),
        ]),
      );
    });
  }

  Widget _buildBlock(_PositionedSchedule p, double columnWidth) {
    final schedule = p.schedule;
    final startMinutes = _timeToMinutes(schedule['start_time'] as String);
    final endMinutes = _timeToMinutes(schedule['end_time'] as String);
    final top = ((startMinutes - (_kStartHour * 60)) / 60) * hourHeight;
    final height = ((endMinutes - startMinutes) / 60) * hourHeight;
    final safeColumnWidth = columnWidth > 0 ? columnWidth : _kWeekDayColumnWidth;
    final laneWidth = safeColumnWidth / p.laneCount;
    final left = p.lane * laneWidth;
    final hasCollision = p.laneCount > 1;

    return Positioned(
      top: top + 1,
      left: left + 1,
      width: (laneWidth - 2).clamp(4.0, safeColumnWidth),
      height: height.clamp(4.0, gridHeight - top),
      child: _ClassBlock(
        subjectName: schedule['subject_name'] as String,
        room: schedule['room'] as String,
        startTime: _formatTime(schedule['start_time'] as String),
        endTime: _formatTime(schedule['end_time'] as String),
        color: colorResolver(schedule['subject_id'] as String),
        isCustomized: schedule['is_customized'] == true,
        hasCollision: hasCollision,
        compact: hasCollision,
        zoom: zoom,
      ),
    );
  }
}

/// Bloque adaptativo: nunca fuerza una altura mínima y recorta el contenido
/// dentro de sus límites, evitando el banner "A RenderFlex overflowed by...".
class _ClassBlock extends StatelessWidget {
  final String subjectName;
  final String room;
  final String startTime;
  final String endTime;
  final Color color;
  final bool isCustomized;
  final bool hasCollision;
  final bool compact;
  final double zoom;

  const _ClassBlock({required this.subjectName, required this.room, required this.startTime, required this.endTime, required this.color, required this.isCustomized, this.hasCollision = false, this.compact = false, this.zoom = 1.0});

  @override
  Widget build(BuildContext context) {
    final titleSize = ((compact ? 8.5 : 10.5) * zoom).clamp(6.0, 16.0);
    final roomSize = ((compact ? 7.5 : 9.5) * zoom).clamp(6.0, 14.0);
    final timeSize = (8.5 * zoom).clamp(6.0, 13.0);

    return ClipRect(
      child: Container(
        width: double.infinity,
        height: double.infinity,
        clipBehavior: Clip.hardEdge,
        padding: EdgeInsets.symmetric(horizontal: compact ? 3 : 6, vertical: compact ? 2 : 4),
        decoration: BoxDecoration(color: color, border: Border.all(color: hasCollision ? AppColors.error : AppColors.border, width: hasCollision ? 2 : 1.5), borderRadius: BorderRadius.circular(3), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(1, 1), blurRadius: 0)]),
        child: LayoutBuilder(builder: (context, constraints) {
          final tiny = constraints.maxHeight < 30 || constraints.maxWidth < 65;
          final showTime = !tiny && !compact && constraints.maxHeight >= 42;
          final textMaxLines = tiny ? 1 : (compact ? 2 : 2);

          return Column(
            mainAxisAlignment: MainAxisAlignment.center,
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(subjectName.toUpperCase(), maxLines: textMaxLines, overflow: TextOverflow.ellipsis, style: TextStyle(fontWeight: FontWeight.w900, fontSize: titleSize, color: AppColors.text, height: 1.0)),
              if (constraints.maxHeight >= 21) Text(room, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(fontWeight: FontWeight.w700, fontSize: roomSize, color: AppColors.text, height: 1.0)),
              if (showTime) Text('$startTime-$endTime', maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(fontWeight: FontWeight.w600, fontSize: timeSize, color: AppColors.text.withOpacity(0.75), height: 1.0)),
            ],
          );
        }),
      ),
    );
  }
}

class _ZoomButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final bool enabled;
  final VoidCallback onPressed;

  const _ZoomButton({required this.icon, required this.tooltip, required this.enabled, required this.onPressed});

  @override
  Widget build(BuildContext context) {
    final color = enabled ? AppColors.text : AppColors.muted.withOpacity(0.35);
    final button = InkWell(
      borderRadius: BorderRadius.circular(4),
      onTap: enabled ? onPressed : null,
      child: Container(
        width: 32,
        height: 32,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: color, width: 2), borderRadius: BorderRadius.circular(4)),
        child: Icon(icon, size: 17, color: color),
      ),
    );
    return Tooltip(message: tooltip, child: button);
  }
}

class _SegmentedDayButton extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;

  const _SegmentedDayButton({required this.label, required this.active, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 12),
        alignment: Alignment.center,
        decoration: BoxDecoration(color: active ? AppColors.accentYellow : AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: active ? const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)] : null),
        child: Text(label, textAlign: TextAlign.center, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w900, letterSpacing: 0.3, color: AppColors.text)),
      ),
    );
  }
}

class _SquareIconButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onPressed;
  final String? tooltip;

  const _SquareIconButton({required this.icon, required this.onPressed, this.tooltip});

  @override
  Widget build(BuildContext context) {
    final button = InkWell(
      borderRadius: BorderRadius.circular(AppDimens.radius),
      onTap: onPressed,
      child: Container(
        width: 42,
        height: 42,
        alignment: Alignment.center,
        decoration: BoxDecoration(color: AppColors.surface, border: Border.all(color: AppColors.border, width: AppDimens.borderWidth), borderRadius: BorderRadius.circular(AppDimens.radius), boxShadow: const [BoxShadow(color: AppColors.border, offset: Offset(2, 2), blurRadius: 0)]),
        child: Icon(icon, size: 18, color: AppColors.text),
      ),
    );
    return tooltip != null ? Tooltip(message: tooltip!, child: button) : button;
  }
}
