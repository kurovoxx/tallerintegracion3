// Ruta: front/lib/features/academic/schedule_screen.dart
//
// "Ficha de Matriz Universitaria Retro": papel técnico, divisiones de tinta,
// etiquetas de laboratorio, sellos y regla de "AHORA". Consume los tokens
// canónicos de core/theme/app_theme.dart y el catálogo core/widgets/neobrutalism.dart.

import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

enum _ViewMode { byDay, fullWeek }

const List<String> _kWeekDayLabels = [
  'DOM',
  'LUN',
  'MAR',
  'MIÉ',
  'JUE',
  'VIE',
  'SÁB',
];
const List<String> _kWeekDayFullLabels = [
  'Domingo',
  'Lunes',
  'Martes',
  'Miércoles',
  'Jueves',
  'Viernes',
  'Sábado',
];
const List<int> _kBaseWeekdays = [1, 2, 3, 4, 5];
const int _kStartHour = 8;
const int _kEndHour = 20;
const double _kHourHeight = 64;
const double _kWeekDayColumnWidth = 152;
const double _kMinWeekDayColumnWidth = 152;
const double _kHourLabelColumnWidth = 64;
const double _kDayHeaderHeight = 40;
const double _kMinZoom = 0.55;
const double _kMaxZoom = 1.80;
const double _kZoomStep = 0.15;

const List<Color> _kSubjectPalette = [
  AppColors.accentYellow,
  AppColors.subjectBlue,
  AppColors.subjectMint,
  AppColors.subjectPeach,
  AppColors.subjectCoral,
];

Color get _kSoftInk => AppColors.border.withValues(alpha: 0.22);

String _formatTime(int minutes) {
  final hour = (minutes ~/ 60).toString().padLeft(2, '0');
  final minute = (minutes % 60).toString().padLeft(2, '0');
  return '$hour:$minute';
}

String _formatClock(DateTime value) =>
    '${value.hour.toString().padLeft(2, '0')}:${value.minute.toString().padLeft(2, '0')}';

class _ClassSession {
  const _ClassSession({
    required this.id,
    required this.subjectId,
    required this.subjectName,
    required this.code,
    required this.dayOfWeek,
    required this.startMinutes,
    required this.endMinutes,
    required this.room,
    required this.professor,
    required this.modality,
    this.isCustomized = false,
  });

  final String id;
  final String subjectId;
  final String subjectName;
  final String code;
  final int dayOfWeek;
  final int startMinutes;
  final int endMinutes;
  final String room;
  final String professor;
  final String modality;
  final bool isCustomized;

  String get timeLabel =>
      '${_formatTime(startMinutes)} - ${_formatTime(endMinutes)}';
}

List<_ClassSession> _buildMockSessions() => const [
  _ClassSession(
    id: 'sch-1',
    subjectId: 'calc-3',
    subjectName: 'Cálculo III',
    code: 'MAT-301',
    dayOfWeek: 1,
    startMinutes: 600,
    endMinutes: 690,
    room: 'CJP11-204',
    professor: 'M. Soto',
    modality: 'Cátedra',
  ),
  _ClassSession(
    id: 'sch-2',
    subjectId: 'calc-3',
    subjectName: 'Cálculo III',
    code: 'MAT-301',
    dayOfWeek: 3,
    startMinutes: 600,
    endMinutes: 690,
    room: 'CJP11-204',
    professor: 'M. Soto',
    modality: 'Cátedra',
  ),
  _ClassSession(
    id: 'sch-3',
    subjectId: 'taller-3',
    subjectName: 'Taller de Integración III',
    code: 'ING-311',
    dayOfWeek: 2,
    startMinutes: 510,
    endMinutes: 600,
    room: 'CJP11-102',
    professor: 'P. Rojas',
    modality: 'Taller',
  ),
  _ClassSession(
    id: 'sch-4',
    subjectId: 'prog-estructurada',
    subjectName: 'Programación Estructurada',
    code: 'ING-322',
    dayOfWeek: 4,
    startMinutes: 510,
    endMinutes: 600,
    room: 'CJP11-102',
    professor: 'L. Muñoz',
    modality: 'Laboratorio',
  ),
  _ClassSession(
    id: 'sch-5',
    subjectId: 'redes',
    subjectName: 'Redes de Computadores',
    code: 'INF-340',
    dayOfWeek: 4,
    startMinutes: 690,
    endMinutes: 780,
    room: 'CJP11-101',
    professor: 'J. Díaz',
    modality: 'Cátedra',
  ),
  _ClassSession(
    id: 'sch-5b',
    subjectId: 'ayudantia-redes',
    subjectName: 'Ayudantía Redes',
    code: 'INF-340A',
    dayOfWeek: 4,
    startMinutes: 720,
    endMinutes: 780,
    room: 'Lab-3',
    professor: 'C. Vera',
    modality: 'Ayudantía',
    isCustomized: true,
  ),
  _ClassSession(
    id: 'sch-6',
    subjectId: 'seginf',
    subjectName: 'Seguridad Informática',
    code: 'INF-360',
    dayOfWeek: 5,
    startMinutes: 840,
    endMinutes: 1020,
    room: 'CJP11-102',
    professor: 'A. Silva',
    modality: 'Cátedra',
    isCustomized: true,
  ),
  _ClassSession(
    id: 'sch-7',
    subjectId: 'lab-redes-sab',
    subjectName: 'Laboratorio de Redes',
    code: 'INF-341',
    dayOfWeek: 6,
    startMinutes: 540,
    endMinutes: 660,
    room: 'Lab-Redes',
    professor: 'J. Díaz',
    modality: 'Laboratorio',
    isCustomized: true,
  ),
];

class ScheduleScreen extends StatefulWidget {
  const ScheduleScreen({super.key});

  @override
  State<ScheduleScreen> createState() => _ScheduleScreenState();
}

class _ScheduleScreenState extends State<ScheduleScreen> {
  bool _isLoading = true;
  bool _hasError = false;
  bool _didInitViewMode = false;
  List<_ClassSession> _sessions = const [];
  int _selectedDay = 1;
  _ViewMode _viewMode = _ViewMode.fullWeek;
  double _weekZoom = 1.0;
  double? _measuredGridWidth;
  DateTime _now = DateTime.now();
  Timer? _clock;

  @override
  void initState() {
    super.initState();
    _clock = Timer.periodic(const Duration(minutes: 1), (_) {
      if (mounted) setState(() => _now = DateTime.now());
    });
    _loadSchedule();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_didInitViewMode) {
      _didInitViewMode = true;
      if (context.isCompact) _viewMode = _ViewMode.byDay;
    }
  }

  @override
  void dispose() {
    _clock?.cancel();
    super.dispose();
  }

  Future<void> _loadSchedule() async {
    if (_isLoading && _sessions.isNotEmpty) return;

    setState(() {
      _isLoading = true;
      _hasError = false;
      _now = DateTime.now();
    });

    try {
      await Future<void>.delayed(const Duration(milliseconds: 500));
      final sessions = _buildMockSessions();
      final displayed = _displayedDaysFor(sessions);
      final today = _todayPostgres;
      if (!mounted) return;
      setState(() {
        _sessions = sessions;
        _selectedDay = displayed.contains(today) ? today : displayed.first;
        _isLoading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _hasError = true;
      });
    }
  }

  // ---------------------------------------------------------------------------
  // Derivados de datos / tiempo
  // ---------------------------------------------------------------------------

  int get _todayPostgres => _now.weekday % 7;

  List<int> _displayedDaysFor(List<_ClassSession> sessions) {
    final extra = sessions
        .map((s) => s.dayOfWeek)
        .where((d) => d == 0 || d == 6)
        .toSet();
    return ({..._kBaseWeekdays, ...extra}.toList()..sort());
  }

  List<int> get _displayedDays => _displayedDaysFor(_sessions);

  List<_ClassSession> _sessionsForDay(int day) {
    final list = _sessions.where((s) => s.dayOfWeek == day).toList()
      ..sort((a, b) => a.startMinutes.compareTo(b.startMinutes));
    return list;
  }

  Color _colorForSubject(String subjectId) =>
      _kSubjectPalette[subjectId.hashCode.abs() % _kSubjectPalette.length];

  DateTime _dateForDay(int day) {
    final base = DateTime(_now.year, _now.month, _now.day);
    return base.add(Duration(days: day - _todayPostgres));
  }

  String _dateLabelFor(int day) =>
      _dateForDay(day).day.toString().padLeft(2, '0');

  String get _academicTermLabel =>
      'Semestre ${_now.year}-${_now.month >= 7 ? 2 : 1}';

  double? _nowOffsetInGrid(double hourHeight) {
    final minutes = _now.hour * 60 + _now.minute;
    final start = _kStartHour * 60;
    final end = _kEndHour * 60;
    if (minutes < start || minutes > end) return null;
    return (minutes - start) / 60 * hourHeight;
  }

  // ---------------------------------------------------------------------------
  // Acciones
  // ---------------------------------------------------------------------------

  void _changeZoom(double delta) {
    setState(() => _weekZoom = (_weekZoom + delta).clamp(_kMinZoom, _kMaxZoom));
  }

  void _fitToScreen() {
    final days = _displayedDays.length;
    final width = _measuredGridWidth;
    final scrollable = _viewMode == _ViewMode.fullWeek && !context.isExpanded;
    if (!scrollable || days == 0 || width == null || width <= 0) {
      setState(() => _weekZoom = 1.0);
      return;
    }
    final naturalWidth =
        days * _kWeekDayColumnWidth +
        math.max(0, days - 1) * AppDimens.borderWidth;
    setState(
      () => _weekZoom = (width / naturalWidth).clamp(_kMinZoom, _kMaxZoom),
    );
  }

  void _handleDaySwipe(DragEndDetails details) {
    final velocity = details.primaryVelocity ?? 0;
    if (velocity.abs() < 120) return;
    final days = _displayedDays;
    final index = days.indexOf(_selectedDay);
    if (index == -1) return;
    final next = (index + (velocity < 0 ? 1 : -1)).clamp(0, days.length - 1);
    if (next != index) setState(() => _selectedDay = days[next]);
  }

  void _selectDay(int day) => setState(() => _selectedDay = day);

  void _setViewMode(_ViewMode mode) => setState(() => _viewMode = mode);

  Future<void> _openClassDetail(_ClassSession session) {
    return showDialog<void>(
      context: context,
      barrierColor: AppColors.scrim,
      builder: (_) => _ClassDetailDialog(
        session: session,
        color: _colorForSubject(session.subjectId),
        dayLabel: _kWeekDayFullLabels[session.dayOfWeek],
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Build
  // ---------------------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(child: _hasError ? _buildErrorState() : _buildContent()),
    );
  }

  Widget _buildContent() {
    final breakpoint = context.breakpoint;
    final compact = breakpoint == AppBreakpoint.compact;
    return MaxWidthContainer(
      maxWidth: AppDimens.contentMaxWidth,
      padding: EdgeInsets.fromLTRB(
        compact ? AppDimens.spaceLg : AppDimens.spaceXl,
        AppDimens.spaceXl,
        compact ? AppDimens.spaceLg : AppDimens.spaceXl,
        AppDimens.spaceHero,
      ),
      child: SingleChildScrollView(
        physics: const BouncingScrollPhysics(),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildHeader(breakpoint),
            const SizedBox(height: AppDimens.spaceXl),
            _buildControlDeck(breakpoint),
            const SizedBox(height: AppDimens.spaceLg),
            if (_isLoading)
              _buildLoadingCard()
            else if (_viewMode == _ViewMode.fullWeek)
              _buildWeekPanel()
            else
              _buildDayPanel(),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(AppBreakpoint breakpoint) {
    final canPop = Navigator.of(context).canPop();
    final compact = breakpoint == AppBreakpoint.compact;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (canPop) ...[
          NeobrutalistIconButton(
            icon: Icons.arrow_back_rounded,
            tooltip: 'Volver',
            onPressed: () => Navigator.of(context).pop(),
          ),
          const SizedBox(width: AppDimens.spaceMd),
        ],
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const _StampTag(
                label: 'Ficha de matriz universitaria',
                icon: Icons.grid_on_rounded,
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                'HORARIO SEMESTRAL',
                style: TextStyle(
                  color: AppColors.text,
                  fontWeight: FontWeight.w900,
                  fontSize: compact ? 24 : 30,
                  letterSpacing: -0.5,
                  height: 1.0,
                ),
              ),
              const SizedBox(height: AppDimens.spaceSm),
              Text(
                'Bloques, salas y modalidad del semestre en curso.',
                style: TextStyle(
                  color: AppColors.mutedStrong,
                  fontWeight: FontWeight.w700,
                  fontSize: compact ? 12.5 : 13.5,
                  height: 1.3,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: AppDimens.spaceMd),
        Column(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            if (!compact) ...[
              _StampTag(
                label: _academicTermLabel,
                icon: Icons.verified_rounded,
                angle: 0.02,
              ),
              const SizedBox(height: AppDimens.spaceSm),
            ],
            NeobrutalistIconButton(
              icon: Icons.refresh_rounded,
              tooltip: 'Actualizar horario',
              onPressed: _loadSchedule,
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildControlDeck(AppBreakpoint breakpoint) {
    final compact = breakpoint == AppBreakpoint.compact;
    return Container(
      decoration: _panelDecoration,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _SheetHeader(
            index: 'CTRL',
            title: 'Panel de control',
            icon: Icons.tune_rounded,
            trailing: _SheetStamp(label: '${_displayedDays.length} días'),
          ),
          Padding(
            padding: EdgeInsets.all(
              compact ? AppDimens.spaceMd : AppDimens.spaceLg,
            ),
            child: compact ? _buildCompactControls() : _buildWideControls(),
          ),
        ],
      ),
    );
  }

  Widget _buildWideControls() {
    return Row(
      children: [
        _buildViewChips(),
        const SizedBox(width: AppDimens.spaceLg),
        Container(
          width: AppDimens.borderWidth,
          height: 34,
          color: AppColors.border,
        ),
        const SizedBox(width: AppDimens.spaceLg),
        Expanded(
          child: _viewMode == _ViewMode.byDay
              ? _buildDaySelector(scrollable: false, compact: false)
              : Text(
                  'Matriz completa · ${_displayedDays.length} columnas',
                  style: const TextStyle(
                    fontFamily: 'monospace',
                    fontSize: 11,
                    fontWeight: FontWeight.w800,
                    letterSpacing: 0.5,
                    color: AppColors.mutedStrong,
                  ),
                ),
        ),
        const SizedBox(width: AppDimens.spaceLg),
        _buildZoomBar(),
      ],
    );
  }

  Widget _buildCompactControls() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: _buildViewChip(
                _ViewMode.fullWeek,
                'Semana',
                Icons.calendar_view_week_rounded,
                compact: true,
              ),
            ),
            const SizedBox(width: AppDimens.spaceSm),
            Expanded(
              child: _buildViewChip(
                _ViewMode.byDay,
                'Por día',
                Icons.calendar_today_rounded,
                compact: true,
              ),
            ),
          ],
        ),
        const SizedBox(height: AppDimens.spaceMd),
        if (_viewMode == _ViewMode.byDay) ...[
          _buildDaySelector(scrollable: true, compact: true),
          const SizedBox(height: AppDimens.spaceMd),
        ],
        _buildZoomBar(expand: true),
      ],
    );
  }

  Widget _buildViewChips() {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        _buildViewChip(
          _ViewMode.fullWeek,
          'Semana',
          Icons.calendar_view_week_rounded,
        ),
        const SizedBox(width: AppDimens.spaceSm),
        _buildViewChip(
          _ViewMode.byDay,
          'Por día',
          Icons.calendar_today_rounded,
        ),
      ],
    );
  }

  Widget _buildViewChip(
    _ViewMode mode,
    String label,
    IconData icon, {
    bool compact = false,
  }) {
    return _MatrixChip(
      label: label,
      icon: icon,
      active: _viewMode == mode,
      compact: compact,
      onTap: () => _setViewMode(mode),
    );
  }

  Widget _buildDaySelector({required bool scrollable, required bool compact}) {
    final days = _displayedDays;
    final chips = <Widget>[
      for (final day in days)
        _MatrixChip(
          label: '${_kWeekDayLabels[day]} ${_dateLabelFor(day)}',
          active: _selectedDay == day,
          compact: compact,
          highlight: day == _todayPostgres,
          onTap: () => _selectDay(day),
        ),
    ];

    if (!scrollable) {
      return Wrap(
        spacing: AppDimens.spaceSm,
        runSpacing: AppDimens.spaceSm,
        children: chips,
      );
    }
    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      physics: const BouncingScrollPhysics(),
      child: Row(
        children: [
          for (final chip in chips) ...[
            chip,
            const SizedBox(width: AppDimens.spaceSm),
          ],
        ],
      ),
    );
  }

  Widget _buildZoomBar({bool expand = false}) {
    final percentage = (_weekZoom * 100).round();
    return Row(
      mainAxisSize: expand ? MainAxisSize.max : MainAxisSize.min,
      children: [
        if (expand) ...[
          const Icon(Icons.zoom_in_rounded, size: 16, color: AppColors.text),
          const SizedBox(width: AppDimens.spaceSm),
          const Text(
            'ZOOM',
            style: TextStyle(
              fontSize: 10.5,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: AppColors.text,
            ),
          ),
          const Spacer(),
        ],
        NeobrutalistIconButton(
          icon: Icons.remove_rounded,
          tooltip: 'Alejar',
          size: 34,
          onPressed: _weekZoom > _kMinZoom
              ? () => _changeZoom(-_kZoomStep)
              : null,
        ),
        const SizedBox(width: AppDimens.spaceSm),
        Container(
          width: 60,
          padding: const EdgeInsets.symmetric(vertical: 9),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: AppColors.bg,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.badge,
          ),
          child: Text(
            '$percentage%',
            style: const TextStyle(
              fontFamily: 'monospace',
              fontSize: 11.5,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
        ),
        const SizedBox(width: AppDimens.spaceSm),
        NeobrutalistIconButton(
          icon: Icons.add_rounded,
          tooltip: 'Acercar',
          size: 34,
          onPressed: _weekZoom < _kMaxZoom
              ? () => _changeZoom(_kZoomStep)
              : null,
        ),
        const SizedBox(width: AppDimens.spaceSm),
        NeobrutalistIconButton(
          icon: Icons.fit_screen_rounded,
          tooltip: 'Ajustar a pantalla',
          size: 34,
          onPressed: _fitToScreen,
        ),
      ],
    );
  }

  // ---------------------------------------------------------------------------
  // Matriz
  // ---------------------------------------------------------------------------

  BoxDecoration get _panelDecoration => BoxDecoration(
    color: AppColors.surface,
    border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
    borderRadius: BorderRadius.circular(AppDimens.radius),
    boxShadow: AppShadows.card,
  );

  BoxDecoration get _matrixFrame => BoxDecoration(
    border: Border.all(color: AppColors.border, width: AppDimens.borderWidth),
  );

  Widget _buildWeekPanel() {
    final days = _displayedDays;
    final expanded = context.isExpanded;
    final hourHeight = _kHourHeight * _weekZoom;
    final gridHeight = (_kEndHour - _kStartHour) * hourHeight;
    final nowOffset = _nowOffsetInGrid(hourHeight);
    final showNow = nowOffset != null && days.contains(_todayPostgres);
    final percentage = (_weekZoom * 100).round();

    return Container(
      decoration: _panelDecoration,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _SheetHeader(
            index: 'MTZ',
            title: 'Matriz semanal',
            icon: Icons.table_chart_rounded,
            trailing: _SheetStamp(
              label: expanded
                  ? '${_hourRangeLabel()} · escala $percentage%'
                  : '${days.length} columnas',
            ),
          ),
          Padding(
            padding: EdgeInsets.all(
              expanded ? AppDimens.spaceLg : AppDimens.spaceMd,
            ),
            child: Container(
              decoration: _matrixFrame,
              child: expanded
                  ? _buildExpandedWeekGrid(
                      days,
                      hourHeight,
                      gridHeight,
                      showNow ? nowOffset : null,
                    )
                  : _buildScrollableWeekGrid(
                      days,
                      hourHeight,
                      gridHeight,
                      showNow ? nowOffset : null,
                    ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildDayPanel() {
    final hourHeight = _kHourHeight * _weekZoom;
    final gridHeight = (_kEndHour - _kStartHour) * hourHeight;
    final nowOffset = _nowOffsetInGrid(hourHeight);
    final sessions = _sessionsForDay(_selectedDay);
    final isToday = _selectedDay == _todayPostgres;
    final showNow = nowOffset != null && isToday;

    return GestureDetector(
      onHorizontalDragEnd: _handleDaySwipe,
      child: Container(
        decoration: _panelDecoration,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _SheetHeader(
              index: 'DÍA',
              title:
                  '${_kWeekDayFullLabels[_selectedDay]} ${_dateLabelFor(_selectedDay)}',
              icon: Icons.today_rounded,
              trailing: _SheetStamp(
                label: sessions.isEmpty
                    ? 'día libre'
                    : '${sessions.length} bloques',
              ),
            ),
            Padding(
              padding: EdgeInsets.all(
                context.isCompact ? AppDimens.spaceMd : AppDimens.spaceLg,
              ),
              child: Container(
                decoration: _matrixFrame,
                child: SizedBox(
                  height: _kDayHeaderHeight + gridHeight,
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      SizedBox(
                        width: _kHourLabelColumnWidth,
                        child: _HourRail(
                          hourHeight: hourHeight,
                          gridHeight: gridHeight,
                        ),
                      ),
                      Container(
                        width: AppDimens.borderWidth,
                        color: AppColors.border,
                      ),
                      Expanded(
                        child: Stack(
                          clipBehavior: Clip.none,
                          children: [
                            Column(
                              crossAxisAlignment: CrossAxisAlignment.stretch,
                              children: [
                                _DayHeaderCell(
                                  label: _kWeekDayLabels[_selectedDay],
                                  dateLabel: _dateLabelFor(_selectedDay),
                                  isToday: isToday,
                                  active: true,
                                ),
                                _DayGrid(
                                  sessions: sessions,
                                  hourHeight: hourHeight,
                                  gridHeight: gridHeight,
                                  detailed: true,
                                  colorResolver: _colorForSubject,
                                  onBlockTap: _openClassDetail,
                                ),
                              ],
                            ),
                            if (showNow) ...[
                              Positioned(
                                top: _kDayHeaderHeight + nowOffset,
                                left: 0,
                                right: 0,
                                child: const _NowRule(),
                              ),
                              Positioned(
                                top: (_kDayHeaderHeight + nowOffset - 12).clamp(
                                  0.0,
                                  _kDayHeaderHeight + gridHeight - 24,
                                ),
                                left: 0,
                                child: _NowBadge(time: _formatClock(_now)),
                              ),
                            ],
                            if (sessions.isEmpty)
                              Positioned(
                                top: _kDayHeaderHeight,
                                left: 0,
                                right: 0,
                                bottom: 0,
                                child: const _EmptyDayStamp(),
                              ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildExpandedWeekGrid(
    List<int> days,
    double hourHeight,
    double gridHeight,
    double? nowOffset,
  ) {
    return SizedBox(
      height: _kDayHeaderHeight + gridHeight,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: _kHourLabelColumnWidth,
            child: _HourRail(hourHeight: hourHeight, gridHeight: gridHeight),
          ),
          Container(width: AppDimens.borderWidth, color: AppColors.border),
          Expanded(
            child: LayoutBuilder(
              builder: (context, constraints) {
                // Nunca comprimir los días a anchos ilegibles: si el ancho
                // disponible no alcanza el mínimo por columna, la fila lo
                // conserva y se recorre con scroll horizontal.
                final minWidth = days.length * _kMinWeekDayColumnWidth;
                final needsScroll = constraints.maxWidth < minWidth;
                if (needsScroll) {
                  return SingleChildScrollView(
                    scrollDirection: Axis.horizontal,
                    physics: const BouncingScrollPhysics(),
                    child: SizedBox(
                      width: minWidth,
                      child: _buildWeekColumnsStack(
                        days: days,
                        hourHeight: hourHeight,
                        gridHeight: gridHeight,
                        nowOffset: nowOffset,
                        expandColumns: false,
                      ),
                    ),
                  );
                }
                return _buildWeekColumnsStack(
                  days: days,
                  hourHeight: hourHeight,
                  gridHeight: gridHeight,
                  nowOffset: nowOffset,
                  expandColumns: true,
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildWeekColumnsStack({
    required List<int> days,
    required double hourHeight,
    required double gridHeight,
    required double? nowOffset,
    required bool expandColumns,
  }) {
    return Stack(
      clipBehavior: Clip.none,
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (int i = 0; i < days.length; i++)
              if (expandColumns)
                Expanded(
                  child: _buildDayColumn(
                    days[i],
                    hourHeight,
                    gridHeight,
                    withLeftBorder: i > 0,
                  ),
                )
              else
                SizedBox(
                  width: _kMinWeekDayColumnWidth,
                  child: _buildDayColumn(
                    days[i],
                    hourHeight,
                    gridHeight,
                    withLeftBorder: i > 0,
                  ),
                ),
          ],
        ),
        if (nowOffset != null) ...[
          Positioned(
            top: _kDayHeaderHeight + nowOffset,
            left: 0,
            right: 0,
            child: const _NowRule(),
          ),
          Positioned(
            top: (_kDayHeaderHeight + nowOffset - 12).clamp(
              0.0,
              _kDayHeaderHeight + gridHeight - 24,
            ),
            left: 0,
            child: _NowBadge(time: _formatClock(_now)),
          ),
        ],
      ],
    );
  }

  Widget _buildDayColumn(
    int day,
    double hourHeight,
    double gridHeight, {
    required bool withLeftBorder,
  }) {
    return Container(
      decoration: withLeftBorder
          ? const BoxDecoration(
              border: Border(
                left: BorderSide(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
              ),
            )
          : null,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _DayHeaderCell(
            label: _kWeekDayLabels[day],
            dateLabel: _dateLabelFor(day),
            isToday: day == _todayPostgres,
            active: _selectedDay == day,
            onTap: () {
              _selectDay(day);
              _setViewMode(_ViewMode.byDay);
            },
          ),
          _DayGrid(
            sessions: _sessionsForDay(day),
            hourHeight: hourHeight,
            gridHeight: gridHeight,
            detailed: false,
            colorResolver: _colorForSubject,
            onBlockTap: _openClassDetail,
          ),
        ],
      ),
    );
  }

  Widget _buildScrollableWeekGrid(
    List<int> days,
    double hourHeight,
    double gridHeight,
    double? nowOffset,
  ) {
    final dayWidth = _kWeekDayColumnWidth * _weekZoom;
    final divider = AppDimens.borderWidth;
    final totalWidth =
        days.length * dayWidth + math.max(0, days.length - 1) * divider;

    return SizedBox(
      height: _kDayHeaderHeight + gridHeight,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: _kHourLabelColumnWidth,
            child: _HourRail(hourHeight: hourHeight, gridHeight: gridHeight),
          ),
          Container(width: divider, color: AppColors.border),
          Expanded(
            child: LayoutBuilder(
              builder: (context, constraints) {
                _measuredGridWidth = constraints.maxWidth;
                return SingleChildScrollView(
                  scrollDirection: Axis.horizontal,
                  physics: const BouncingScrollPhysics(),
                  child: SizedBox(
                    width: totalWidth,
                    child: Stack(
                      clipBehavior: Clip.none,
                      children: [
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            for (int i = 0; i < days.length; i++) ...[
                              SizedBox(
                                width: dayWidth,
                                child: Column(
                                  crossAxisAlignment:
                                      CrossAxisAlignment.stretch,
                                  children: [
                                    _DayHeaderCell(
                                      label: _kWeekDayLabels[days[i]],
                                      dateLabel: _dateLabelFor(days[i]),
                                      isToday: days[i] == _todayPostgres,
                                      active: _selectedDay == days[i],
                                      onTap: () {
                                        _selectDay(days[i]);
                                        _setViewMode(_ViewMode.byDay);
                                      },
                                    ),
                                    _DayGrid(
                                      sessions: _sessionsForDay(days[i]),
                                      hourHeight: hourHeight,
                                      gridHeight: gridHeight,
                                      detailed: dayWidth >= 170,
                                      colorResolver: _colorForSubject,
                                      onBlockTap: _openClassDetail,
                                    ),
                                  ],
                                ),
                              ),
                              if (i != days.length - 1)
                                Container(
                                  width: divider,
                                  color: AppColors.border,
                                ),
                            ],
                          ],
                        ),
                        if (nowOffset != null) ...[
                          Positioned(
                            top: _kDayHeaderHeight + nowOffset,
                            left: 0,
                            right: 0,
                            child: const _NowRule(),
                          ),
                          Positioned(
                            top: (_kDayHeaderHeight + nowOffset - 12).clamp(
                              0.0,
                              _kDayHeaderHeight + gridHeight - 24,
                            ),
                            left: 0,
                            child: _NowBadge(time: _formatClock(_now)),
                          ),
                        ],
                      ],
                    ),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  String _hourRangeLabel() =>
      '${_kStartHour.toString().padLeft(2, '0')}:00–${_kEndHour.toString().padLeft(2, '0')}:00';

  // ---------------------------------------------------------------------------
  // Estados
  // ---------------------------------------------------------------------------

  Widget _buildLoadingCard() {
    return Container(
      height: 300,
      decoration: _panelDecoration,
      child: const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            SizedBox(
              width: 34,
              height: 34,
              child: CircularProgressIndicator(
                color: AppColors.text,
                strokeWidth: 3,
              ),
            ),
            SizedBox(height: AppDimens.spaceLg),
            Text(
              'CARGANDO MATRIZ…',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.8,
                color: AppColors.text,
              ),
            ),
            SizedBox(height: AppDimens.spaceXs),
            Text(
              'Leyendo bloques del semestre',
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildErrorState() {
    return MaxWidthContainer(
      padding: const EdgeInsets.all(AppDimens.spaceXl),
      child: Center(
        child: Container(
          padding: const EdgeInsets.all(AppDimens.spaceXxl),
          decoration: _panelDecoration,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(
                Icons.error_outline_rounded,
                size: 44,
                color: AppColors.error,
              ),
              const SizedBox(height: AppDimens.spaceMd),
              const Text(
                'NO SE PUDO CARGAR TU HORARIO',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w900,
                  letterSpacing: -0.2,
                  color: AppColors.text,
                ),
              ),
              const SizedBox(height: AppDimens.spaceSm),
              const Text(
                'Revisa tu conexión e inténtalo nuevamente.',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: AppColors.mutedStrong,
                ),
              ),
              const SizedBox(height: AppDimens.spaceXl),
              NeobrutalistButton(
                label: 'Reintentar',
                icon: Icons.refresh_rounded,
                variant: NeobrutalistButtonVariant.accent,
                onPressed: _loadSchedule,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Resolución de choques (carriles)
// ---------------------------------------------------------------------------

class _LaneSession {
  const _LaneSession({
    required this.session,
    required this.lane,
    required this.laneCount,
  });

  final _ClassSession session;
  final int lane;
  final int laneCount;
}

List<_LaneSession> _resolveLanes(List<_ClassSession> sessions) {
  final sorted = [...sessions]
    ..sort((a, b) => a.startMinutes.compareTo(b.startMinutes));
  final result = <_LaneSession>[];
  var cluster = <_ClassSession>[];
  var clusterEnd = -1;

  void flush() {
    if (cluster.isEmpty) return;
    final laneEndTimes = <int>[];
    final laneOf = <_ClassSession, int>{};
    for (final session in cluster) {
      int? freeLane;
      for (var i = 0; i < laneEndTimes.length; i++) {
        if (laneEndTimes[i] <= session.startMinutes) {
          freeLane = i;
          break;
        }
      }
      if (freeLane == null) {
        freeLane = laneEndTimes.length;
        laneEndTimes.add(session.endMinutes);
      } else {
        laneEndTimes[freeLane] = session.endMinutes;
      }
      laneOf[session] = freeLane;
    }
    for (final session in cluster) {
      result.add(
        _LaneSession(
          session: session,
          lane: laneOf[session]!,
          laneCount: laneEndTimes.length,
        ),
      );
    }
    cluster = <_ClassSession>[];
    clusterEnd = -1;
  }

  for (final session in sorted) {
    if (cluster.isEmpty || session.startMinutes < clusterEnd) {
      cluster.add(session);
      clusterEnd = math.max(clusterEnd, session.endMinutes);
    } else {
      flush();
      cluster.add(session);
      clusterEnd = session.endMinutes;
    }
  }
  flush();
  return result;
}

// ---------------------------------------------------------------------------
// Grilla diaria
// ---------------------------------------------------------------------------

class _DayGrid extends StatelessWidget {
  const _DayGrid({
    required this.sessions,
    required this.hourHeight,
    required this.gridHeight,
    required this.detailed,
    required this.colorResolver,
    required this.onBlockTap,
  });

  final List<_ClassSession> sessions;
  final double hourHeight;
  final double gridHeight;
  final bool detailed;
  final Color Function(String subjectId) colorResolver;
  final ValueChanged<_ClassSession> onBlockTap;

  @override
  Widget build(BuildContext context) {
    final totalHours = _kEndHour - _kStartHour;
    final positioned = _resolveLanes(sessions);

    return LayoutBuilder(
      builder: (context, constraints) {
        final columnWidth = constraints.maxWidth.isFinite
            ? constraints.maxWidth
            : _kWeekDayColumnWidth;
        return SizedBox(
          height: gridHeight,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              for (int i = 1; i < totalHours; i++)
                Positioned(
                  top: i * hourHeight - AppDimens.borderWidth / 2,
                  left: 0,
                  right: 0,
                  height: AppDimens.borderWidth,
                  child: const ColoredBox(color: AppColors.border),
                ),
              for (int i = 0; i < totalHours; i++)
                Positioned(
                  top: (i + 0.5) * hourHeight - 0.75,
                  left: 0,
                  right: 0,
                  height: 1.5,
                  child: ColoredBox(color: _kSoftInk),
                ),
              for (final lane in positioned) _buildBlock(lane, columnWidth),
            ],
          ),
        );
      },
    );
  }

  Widget _buildBlock(_LaneSession lane, double columnWidth) {
    final session = lane.session;
    final top = ((session.startMinutes - _kStartHour * 60) / 60) * hourHeight;
    final height =
        ((session.endMinutes - session.startMinutes) / 60) * hourHeight;
    final laneWidth = columnWidth / lane.laneCount;
    final left = lane.lane * laneWidth;

    final maxWidth = math.max(6.0, laneWidth);
    final maxHeight = math.max(6.0, gridHeight - top - 1);
    return Positioned(
      top: top + 1,
      left: left + 1,
      width: (laneWidth - 5).clamp(6.0, maxWidth),
      height: (height - 5).clamp(6.0, maxHeight),
      child: _ClassBlock(
        session: session,
        color: colorResolver(session.subjectId),
        collision: lane.laneCount > 1,
        detailed: detailed,
        zoom: hourHeight / _kHourHeight,
        onTap: () => onBlockTap(session),
      ),
    );
  }
}

class _ClassBlock extends StatefulWidget {
  const _ClassBlock({
    required this.session,
    required this.color,
    required this.collision,
    required this.detailed,
    required this.zoom,
    required this.onTap,
  });

  final _ClassSession session;
  final Color color;
  final bool collision;
  final bool detailed;
  final double zoom;
  final VoidCallback onTap;

  @override
  State<_ClassBlock> createState() => _ClassBlockState();
}

class _ClassBlockState extends State<_ClassBlock> {
  bool _pressed = false;
  bool _hovered = false;

  void _setPressed(bool value) {
    if (_pressed != value) setState(() => _pressed = value);
  }

  void _setHovered(bool value) {
    if (_hovered != value) setState(() => _hovered = value);
  }

  @override
  Widget build(BuildContext context) {
    final session = widget.session;
    final shadow = _pressed
        ? const <BoxShadow>[]
        : (_hovered
              ? AppShadows.card
              : (widget.collision ? AppShadows.badge : AppShadows.button));

    return Semantics(
      button: true,
      label:
          '${session.subjectName}, ${session.timeLabel}, sala ${session.room}',
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
        onEnter: (_) => _setHovered(true),
        onExit: (_) => _setHovered(false),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: (_) => _setPressed(true),
          onTapUp: (_) => _setPressed(false),
          onTapCancel: () => _setPressed(false),
          onTap: widget.onTap,
          child: AnimatedContainer(
            duration: AppMotion.press,
            curve: AppMotion.standard,
            transform: Matrix4.translationValues(
              _pressed ? AppShadows.offsetButton.dx : 0,
              _pressed ? AppShadows.offsetButton.dy : 0,
              0,
            ),
            decoration: BoxDecoration(
              color: widget.color,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: shadow,
            ),
            child: ClipRect(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 4),
                child: LayoutBuilder(
                  builder: (context, constraints) {
                    final height = constraints.maxHeight;
                    final width = constraints.maxWidth;
                    final showCode = height >= 30;
                    final showTime = height >= 46;
                    final showTags = height >= 64 && width >= 110;
                    final showProfessor =
                        widget.detailed && showTags && width >= 170;
                    final nameSize = ((showTags ? 11.0 : 10.0) * widget.zoom)
                        .clamp(6.5, 15.0);
                    final codeSize = (9.0 * widget.zoom).clamp(6.0, 11.5);
                    final timeSize = (9.5 * widget.zoom).clamp(6.0, 12.0);

                    return ClipRect(
                      child: OverflowBox(
                        alignment: Alignment.topLeft,
                        minWidth: 0,
                        maxWidth: constraints.maxWidth,
                        minHeight: 0,
                        maxHeight: double.infinity,
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              session.subjectName.toUpperCase(),
                              maxLines: showCode ? 2 : 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                fontWeight: FontWeight.w900,
                                fontSize: nameSize,
                                height: 1.05,
                                letterSpacing: -0.2,
                                color: AppColors.text,
                              ),
                            ),
                            if (showCode) ...[
                              const SizedBox(height: 2),
                              Text(
                                session.code.toUpperCase(),
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(
                                  fontFamily: 'monospace',
                                  fontWeight: FontWeight.w900,
                                  fontSize: codeSize,
                                  letterSpacing: 0.4,
                                  color: AppColors.text.withValues(alpha: 0.78),
                                ),
                              ),
                            ],
                            if (showTime) ...[
                              const SizedBox(height: 3),
                              Text(
                                session.timeLabel,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(
                                  fontFamily: 'monospace',
                                  fontWeight: FontWeight.w700,
                                  fontSize: timeSize,
                                  letterSpacing: 0.2,
                                  color: AppColors.text.withValues(alpha: 0.85),
                                ),
                              ),
                            ],
                            if (showTags) ...[
                              const SizedBox(height: 4),
                              Wrap(
                                spacing: 4,
                                runSpacing: 3,
                                children: [
                                  _MiniTag(label: session.room),
                                  _MiniTag(label: session.modality),
                                  if (showProfessor)
                                    _MiniTag(label: session.professor),
                                ],
                              ),
                            ],
                          ],
                        ),
                      ),
                    );
                  },
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _MiniTag extends StatelessWidget {
  const _MiniTag({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: FittedBox(
        fit: BoxFit.scaleDown,
        alignment: Alignment.centerLeft,
        child: Text(
          label.toUpperCase(),
          maxLines: 1,
          softWrap: false,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(
            fontSize: 9,
            fontWeight: FontWeight.w900,
            letterSpacing: 0.4,
            color: AppColors.text,
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Regla y etiquetas de tiempo
// ---------------------------------------------------------------------------

class _NowRule extends StatelessWidget {
  const _NowRule();

  @override
  Widget build(BuildContext context) {
    return const SizedBox(height: 3, child: ColoredBox(color: AppColors.error));
  }
}

class _NowBadge extends StatelessWidget {
  const _NowBadge({required this.time});

  final String time;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: AppColors.error,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        'AHORA $time',
        style: const TextStyle(
          fontFamily: 'monospace',
          fontSize: 9.5,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.6,
          color: AppColors.surface,
        ),
      ),
    );
  }
}

class _HourRail extends StatelessWidget {
  const _HourRail({required this.hourHeight, required this.gridHeight});

  final double hourHeight;
  final double gridHeight;

  @override
  Widget build(BuildContext context) {
    final totalHours = _kEndHour - _kStartHour;
    final labelSize = (10.5 * (hourHeight / _kHourHeight)).clamp(8.0, 12.5);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Container(
          height: _kDayHeaderHeight,
          alignment: Alignment.center,
          decoration: const BoxDecoration(
            color: AppColors.surfaceLow,
            border: Border(
              bottom: BorderSide(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
            ),
          ),
          child: const Text(
            'HRS',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w900,
              letterSpacing: 0.6,
              color: AppColors.text,
            ),
          ),
        ),
        SizedBox(
          height: gridHeight,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              for (int i = 0; i < totalHours; i++)
                Positioned(
                  top: (i + 0.5) * hourHeight - 0.75,
                  right: 0,
                  width: 16,
                  child: Container(height: 1.5, color: _kSoftInk),
                ),
              for (int i = 1; i < totalHours; i++)
                Positioned(
                  top: i * hourHeight - AppDimens.borderWidth / 2,
                  left: 0,
                  right: 0,
                  height: AppDimens.borderWidth,
                  child: const ColoredBox(color: AppColors.border),
                ),
              for (int i = 0; i < totalHours; i++)
                Positioned(
                  top: math.max(0.0, i * hourHeight - 10),
                  right: 5,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 4,
                      vertical: 2,
                    ),
                    decoration: BoxDecoration(
                      color: AppColors.surface,
                      border: Border.all(color: AppColors.border, width: 1.5),
                      borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                      boxShadow: AppShadows.badge,
                    ),
                    child: Text(
                      '${(_kStartHour + i).toString().padLeft(2, '0')}:00',
                      style: TextStyle(
                        fontFamily: 'monospace',
                        fontSize: labelSize,
                        fontWeight: FontWeight.w900,
                        height: 1.0,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ],
    );
  }
}

class _DayHeaderCell extends StatelessWidget {
  const _DayHeaderCell({
    required this.label,
    required this.dateLabel,
    required this.isToday,
    required this.active,
    this.onTap,
  });

  final String label;
  final String dateLabel;
  final bool isToday;
  final bool active;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final cell = Container(
      height: _kDayHeaderHeight,
      padding: const EdgeInsets.symmetric(horizontal: 6),
      decoration: BoxDecoration(
        color: isToday
            ? AppColors.accentYellow
            : (active ? AppColors.surfaceLow : AppColors.surface),
        border: const Border(
          bottom: BorderSide(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
        ),
      ),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Flexible(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
                color: AppColors.text,
              ),
            ),
          ),
          const SizedBox(width: 5),
          Flexible(
            child: Text(
              dateLabel,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontFamily: 'monospace',
                fontSize: 10.5,
                fontWeight: FontWeight.w800,
                color: AppColors.text,
              ),
            ),
          ),
          if (isToday) ...[
            const SizedBox(width: 4),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 3, vertical: 1),
              decoration: BoxDecoration(
                color: AppColors.errorDeep,
                borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              ),
              child: const Text(
                'HOY',
                style: TextStyle(
                  fontSize: 7.5,
                  fontWeight: FontWeight.w900,
                  letterSpacing: 0.4,
                  color: AppColors.surface,
                ),
              ),
            ),
          ],
        ],
      ),
    );

    if (onTap == null) return cell;
    return ClickCursor(
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: onTap,
        child: cell,
      ),
    );
  }
}

class _EmptyDayStamp extends StatelessWidget {
  const _EmptyDayStamp();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        padding: const EdgeInsets.symmetric(
          horizontal: AppDimens.spaceXl,
          vertical: AppDimens.spaceLg,
        ),
        decoration: BoxDecoration(
          color: AppColors.surface,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: BorderRadius.circular(AppDimens.radius),
          boxShadow: AppShadows.button,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(
              Icons.event_available_rounded,
              size: 30,
              color: AppColors.text,
            ),
            const SizedBox(height: AppDimens.spaceSm),
            const Text(
              'SIN CLASES PROGRAMADAS',
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.4,
                color: AppColors.text,
              ),
            ),
            const SizedBox(height: AppDimens.spaceXs),
            Text(
              'Día libre en la matriz',
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Piezas de la ficha técnica
// ---------------------------------------------------------------------------

class _SheetHeader extends StatelessWidget {
  const _SheetHeader({
    required this.index,
    required this.title,
    this.icon,
    this.trailing,
  });

  final String index;
  final String title;
  final IconData? icon;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceLg,
        vertical: AppDimens.spaceMd,
      ),
      decoration: const BoxDecoration(
        color: AppColors.surfaceLow,
        border: Border(
          bottom: BorderSide(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
        ),
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(color: AppColors.border, width: 1.5),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
            ),
            child: Text(
              index.toUpperCase(),
              style: const TextStyle(
                fontFamily: 'monospace',
                fontSize: 9.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.8,
                color: AppColors.text,
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          if (icon != null) ...[
            Icon(icon, size: 15, color: AppColors.text),
            const SizedBox(width: AppDimens.spaceXs),
          ],
          Expanded(
            child: Text(
              title.toUpperCase(),
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.6,
                color: AppColors.text,
              ),
            ),
          ),
          if (trailing != null) ...[
            const SizedBox(width: AppDimens.spaceSm),
            trailing!,
          ],
        ],
      ),
    );
  }
}

class _StampTag extends StatelessWidget {
  const _StampTag({required this.label, this.icon, this.angle = -0.02});

  final String label;
  final IconData? icon;
  final double angle;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: angle,
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

class _SheetStamp extends StatelessWidget {
  const _SheetStamp({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
      decoration: BoxDecoration(
        color: AppColors.surface,
        border: Border.all(color: AppColors.border, width: 1.5),
        borderRadius: BorderRadius.circular(AppDimens.radiusChip),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        label.toUpperCase(),
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(
          fontFamily: 'monospace',
          fontSize: 9.5,
          fontWeight: FontWeight.w900,
          letterSpacing: 0.6,
          color: AppColors.text,
        ),
      ),
    );
  }
}

// _MatrixChip es el chip neobrutalista de selección (vista y días): borde de
// 2 px, sombra 'hard2' y estado activo en AppColors.accentYellow.
class _MatrixChip extends StatefulWidget {
  const _MatrixChip({
    required this.label,
    required this.active,
    this.icon,
    this.onTap,
    this.compact = false,
    this.highlight = false,
  });

  final String label;
  final bool active;
  final IconData? icon;
  final VoidCallback? onTap;
  final bool compact;
  final bool highlight;

  @override
  State<_MatrixChip> createState() => _MatrixChipState();
}

class _MatrixChipState extends State<_MatrixChip> {
  bool _pressed = false;
  bool _hovered = false;

  @override
  Widget build(BuildContext context) {
    final interactive = widget.onTap != null;
    final pressed = _pressed && interactive;

    return Semantics(
      button: true,
      selected: widget.active,
      child: MouseRegion(
        cursor: interactive ? SystemMouseCursors.click : MouseCursor.defer,
        onEnter: interactive ? (_) => setState(() => _hovered = true) : null,
        onExit: interactive ? (_) => setState(() => _hovered = false) : null,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: interactive
              ? (_) => setState(() => _pressed = true)
              : null,
          onTapUp: interactive ? (_) => setState(() => _pressed = false) : null,
          onTapCancel: interactive
              ? () => setState(() => _pressed = false)
              : null,
          onTap: widget.onTap,
          child: AnimatedContainer(
            duration: AppMotion.press,
            curve: AppMotion.standard,
            transform: Matrix4.translationValues(
              pressed ? AppShadows.offsetBadge.dx : 0,
              pressed ? AppShadows.offsetBadge.dy : 0,
              0,
            ),
            padding: EdgeInsets.symmetric(
              horizontal: widget.compact ? 9 : 13,
              vertical: widget.compact ? 8 : 10,
            ),
            decoration: BoxDecoration(
              color: widget.active
                  ? AppColors.accentYellow
                  : (_hovered ? AppColors.surfaceLow : AppColors.surface),
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: pressed ? const <BoxShadow>[] : AppShadows.badge,
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                if (widget.highlight) ...[
                  Container(
                    width: 7,
                    height: 7,
                    decoration: BoxDecoration(
                      color: widget.active
                          ? AppColors.errorDeep
                          : AppColors.error,
                      border: Border.all(color: AppColors.border, width: 1),
                      borderRadius: BorderRadius.circular(2),
                    ),
                  ),
                  const SizedBox(width: 5),
                ],
                if (widget.icon != null) ...[
                  Icon(widget.icon, size: 14, color: AppColors.text),
                  const SizedBox(width: 6),
                ],
                Text(
                  widget.label.toUpperCase(),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: widget.compact ? 10.5 : 11.5,
                    fontWeight: FontWeight.w900,
                    letterSpacing: 0.6,
                    color: AppColors.text,
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

// ---------------------------------------------------------------------------
// Detalle modal de clase
// ---------------------------------------------------------------------------

class _ClassDetailDialog extends StatelessWidget {
  const _ClassDetailDialog({
    required this.session,
    required this.color,
    required this.dayLabel,
  });

  final _ClassSession session;
  final Color color;
  final String dayLabel;

  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: Colors.transparent,
      elevation: 0,
      insetPadding: const EdgeInsets.all(AppDimens.spaceXl),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 480),
        child: Container(
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidthThick,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: AppShadows.dialog,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Container(
                padding: const EdgeInsets.fromLTRB(18, 16, 12, 16),
                decoration: BoxDecoration(
                  color: color,
                  border: const Border(
                    bottom: BorderSide(
                      color: AppColors.border,
                      width: AppDimens.borderWidth,
                    ),
                  ),
                ),
                child: Row(
                  children: [
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            session.code.toUpperCase(),
                            style: const TextStyle(
                              fontFamily: 'monospace',
                              fontSize: 10.5,
                              fontWeight: FontWeight.w900,
                              letterSpacing: 1,
                              color: AppColors.text,
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceXs),
                          Text(
                            session.subjectName.toUpperCase(),
                            style: const TextStyle(
                              fontSize: 18,
                              fontWeight: FontWeight.w900,
                              letterSpacing: -0.3,
                              height: 1.05,
                              color: AppColors.text,
                            ),
                          ),
                        ],
                      ),
                    ),
                    NeobrutalistIconButton(
                      icon: Icons.close_rounded,
                      tooltip: 'Cerrar',
                      size: 34,
                      onPressed: () => Navigator.of(context).pop(),
                    ),
                  ],
                ),
              ),
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.all(AppDimens.spaceXl),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      _DetailRow(
                        label: 'Día',
                        value: dayLabel,
                        icon: Icons.calendar_today_rounded,
                      ),
                      _DetailRow(
                        label: 'Horario',
                        value: session.timeLabel,
                        icon: Icons.schedule_rounded,
                        mono: true,
                      ),
                      _DetailRow(
                        label: 'Sala',
                        value: session.room,
                        icon: Icons.meeting_room_rounded,
                      ),
                      _DetailRow(
                        label: 'Profesor',
                        value: session.professor,
                        icon: Icons.person_rounded,
                      ),
                      const SizedBox(height: AppDimens.spaceSm),
                      Wrap(
                        spacing: AppDimens.spaceSm,
                        runSpacing: AppDimens.spaceSm,
                        children: [
                          NeobrutalistBadge(
                            label: session.modality,
                            tone: NeobrutalistTone.accent,
                            icon: Icons.category_rounded,
                          ),
                          NeobrutalistBadge(
                            label: session.room,
                            tone: NeobrutalistTone.info,
                            icon: Icons.meeting_room_rounded,
                          ),
                          if (session.isCustomized)
                            const NeobrutalistBadge(
                              label: 'Personalizado',
                              tone: NeobrutalistTone.pending,
                              icon: Icons.edit_rounded,
                            ),
                        ],
                      ),
                      const SizedBox(height: AppDimens.spaceXl),
                      const Divider(
                        color: AppColors.border,
                        thickness: AppDimens.borderWidth,
                        height: AppDimens.borderWidth,
                      ),
                      const SizedBox(height: AppDimens.spaceMd),
                      const _StampTag(
                        label: 'Registro de matriz · Sigma Academy',
                        icon: Icons.verified_rounded,
                      ),
                    ],
                  ),
                ),
              ),
              const Divider(
                color: AppColors.border,
                thickness: AppDimens.borderWidth,
                height: AppDimens.borderWidth,
              ),
              Padding(
                padding: const EdgeInsets.all(AppDimens.spaceLg),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.end,
                  children: [
                    NeobrutalistButton(
                      label: 'Cerrar ficha',
                      icon: Icons.check_rounded,
                      variant: NeobrutalistButtonVariant.accent,
                      onPressed: () => Navigator.of(context).pop(),
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

class _DetailRow extends StatelessWidget {
  const _DetailRow({
    required this.label,
    required this.value,
    this.icon,
    this.mono = false,
  });

  final String label;
  final String value;
  final IconData? icon;
  final bool mono;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(
            icon ?? Icons.label_rounded,
            size: 15,
            color: AppColors.mutedStrong,
          ),
          const SizedBox(width: AppDimens.spaceSm),
          SizedBox(
            width: 74,
            child: Text(
              label.toUpperCase(),
              style: const TextStyle(
                fontFamily: 'monospace',
                fontSize: 10,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.5,
                color: AppColors.mutedStrong,
              ),
            ),
          ),
          const SizedBox(width: AppDimens.spaceSm),
          Expanded(
            child: Text(
              value,
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w700,
                fontFamily: mono ? 'monospace' : null,
                letterSpacing: mono ? 0.5 : 0,
                color: AppColors.text,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
