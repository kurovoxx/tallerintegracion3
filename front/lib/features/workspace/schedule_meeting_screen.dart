// Ruta: front/lib/features/workspace/schedule_meeting_screen.dart
//
// Formulario "Agendar reunión": ficha neobrutalista retro con tarjeta de
// borde 2.5 px y sombra hard4, campos de tinta, selectores de fecha/hora
// tipo sticker y chips invertidos para los integrantes. Consume los tokens
// canónicos de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

class ScheduleMeetingScreen extends StatefulWidget {
  const ScheduleMeetingScreen({super.key, this.groupId, SocialService? service})
    : _serviceOverride = service;

  final String? groupId;
  final SocialService? _serviceOverride;

  @override
  State<ScheduleMeetingScreen> createState() => _ScheduleMeetingScreenState();
}

class _ScheduleMeetingScreenState extends State<ScheduleMeetingScreen> {
  final _formKey = GlobalKey<FormState>();
  final _titleCtrl = TextEditingController();
  final _descCtrl = TextEditingController();
  DateTime _selectedDate = DateTime.now().add(const Duration(days: 1));
  TimeOfDay _selectedTime = const TimeOfDay(hour: 10, minute: 0);

  late final SocialService _social;
  bool _isSubmitting = false;
  String? _createdMeetingId;
  String? _submitError;
  List<MeetingItem> _upcoming = const [];
  bool _loadingUpcoming = false;

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
    if (_isRealGroup) _loadUpcoming();
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    _descCtrl.dispose();
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  bool get _isRealGroup {
    final id = widget.groupId?.trim() ?? '';
    final uuid = RegExp(
      r'^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$',
    );
    return id.isNotEmpty && uuid.hasMatch(id);
  }

  /// Próximas reuniones desde GET /groups/:id/workspace.meetings.upcoming.
  /// Best-effort: si falla, el formulario sigue funcionando.
  Future<void> _loadUpcoming() async {
    if (!_isRealGroup || !mounted) return;
    setState(() => _loadingUpcoming = true);
    try {
      final ws = await _social.getWorkspace(widget.groupId!.trim());
      if (!mounted) return;
      setState(() => _upcoming = ws.upcomingMeetings);
    } catch (_) {
      // Silencioso: la creación no depende del listado.
    } finally {
      if (mounted) setState(() => _loadingUpcoming = false);
    }
  }

  static String _formatWhen(DateTime when) {
    final local = when.toLocal();
    final date =
        '${local.day.toString().padLeft(2, '0')}/${local.month.toString().padLeft(2, '0')}/${local.year}';
    final time =
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
    return '$date · $time';
  }

  Future<void> _pickDate() async {
    final d = await showDatePicker(
      context: context,
      initialDate: _selectedDate,
      firstDate: DateTime.now(),
      lastDate: DateTime.now().add(const Duration(days: 365)),
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: const ColorScheme.light(primary: AppColors.border),
        ),
        child: child!,
      ),
    );
    if (d != null) setState(() => _selectedDate = d);
  }

  Future<void> _pickTime() async {
    final t = await showTimePicker(
      context: context,
      initialTime: _selectedTime,
      builder: (context, child) => Theme(
        data: Theme.of(context).copyWith(
          colorScheme: const ColorScheme.light(primary: AppColors.border),
        ),
        child: child!,
      ),
    );
    if (t != null) setState(() => _selectedTime = t);
  }

  // POST /groups/:id/meetings {title, description?, scheduled_at}.
  // Solo esos campos tienen efecto en el backend.
  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    if (!_isRealGroup) {
      setState(
        () => _submitError = 'Selecciona un grupo para agendar una reunión.',
      );
      return;
    }
    setState(() {
      _isSubmitting = true;
      _submitError = null;
      _createdMeetingId = null;
    });
    try {
      final when = DateTime(
        _selectedDate.year,
        _selectedDate.month,
        _selectedDate.day,
        _selectedTime.hour,
        _selectedTime.minute,
      ).toUtc();
      final desc = _descCtrl.text.trim();
      final id = await _social.createMeeting(
        groupId: widget.groupId!.trim(),
        title: _titleCtrl.text.trim(),
        description: desc.isEmpty ? null : desc,
        scheduledAtUtc: when,
      );
      if (!mounted) return;
      setState(() => _createdMeetingId = id);
      // Refresca el listado para que la reunión quede visible de inmediato.
      await _loadUpcoming();
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Reunión agendada.'),
          backgroundColor: AppColors.border,
        ),
      );
    } on SocialApiException catch (_) {
      if (!mounted) return;
      setState(() => _submitError = 'No se pudo agendar la reunión.');
    } catch (_) {
      if (!mounted) return;
      setState(() => _submitError = 'No se pudo agendar la reunión.');
    } finally {
      if (mounted) setState(() => _isSubmitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        MediaQuery.of(context).size.width > AppDimens.breakpointDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: SingleChildScrollView(
          physics: const BouncingScrollPhysics(),
          padding: EdgeInsets.symmetric(
            horizontal: isDesktop ? AppDimens.spaceXxl : AppDimens.spaceLg,
            vertical: AppDimens.spaceLg,
          ),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 640),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  _buildHeader(isDesktop),
                  const SizedBox(height: AppDimens.spaceLg),
                  Container(
                    padding: const EdgeInsets.all(AppDimens.spaceXl),
                    decoration: BoxDecoration(
                      color: AppColors.surface,
                      border: Border.all(
                        color: AppColors.border,
                        width: AppDimens.borderWidthAction,
                      ),
                      borderRadius: BorderRadius.circular(AppDimens.radius),
                      boxShadow: AppShadows.card,
                    ),
                    child: Form(
                      key: _formKey,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          const AppFieldLabel('TÍTULO DE LA REUNIÓN'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _titleCtrl,
                              decoration: appInputDecoration(
                                'Ej. Repaso Cálculo II',
                              ),
                              validator: (v) => (v == null || v.trim().isEmpty)
                                  ? 'Requerido'
                                  : null,
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('DESCRIPCIÓN (OPCIONAL)'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _descCtrl,
                              maxLines: 3,
                              decoration: appInputDecoration(
                                'Agenda, temas a revisar...',
                              ),
                              style: const TextStyle(
                                fontWeight: FontWeight.w600,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          Row(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Expanded(
                                child: _buildPickerBox(
                                  label: 'FECHA',
                                  icon: Icons.calendar_month_rounded,
                                  value:
                                      '${_selectedDate.day}/${_selectedDate.month}/${_selectedDate.year}',
                                  onTap: _pickDate,
                                ),
                              ),
                              const SizedBox(width: AppDimens.spaceMd),
                              Expanded(
                                child: _buildPickerBox(
                                  label: 'HORA',
                                  icon: Icons.access_time_rounded,
                                  value: _selectedTime.format(context),
                                  onTap: _pickTime,
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          if (_createdMeetingId != null) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(AppDimens.spaceMd),
                              decoration: BoxDecoration(
                                color: const Color(0xFFE7F6E7),
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: const Text(
                                'Reunión agendada.',
                                style: TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w800,
                                  color: AppColors.text,
                                ),
                              ),
                            ),
                          ],
                          if (_submitError != null) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(AppDimens.spaceMd),
                              decoration: BoxDecoration(
                                color: const Color(0xFFFDE8E8),
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: Text(
                                _submitError!,
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w700,
                                  color: AppColors.text,
                                ),
                              ),
                            ),
                          ],
                          const SizedBox(height: AppDimens.spaceMd),
                          if (!_isRealGroup) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Container(
                              padding: const EdgeInsets.all(10),
                              decoration: BoxDecoration(
                                color: AppColors.bg,
                                border: Border.all(
                                  color: AppColors.border,
                                  width: AppDimens.borderWidth,
                                ),
                                borderRadius: BorderRadius.circular(
                                  AppDimens.radius,
                                ),
                              ),
                              child: const Text(
                                'Selecciona un grupo para agendar una reunión.',
                                style: TextStyle(
                                  fontSize: 11.5,
                                  fontWeight: FontWeight.w700,
                                  color: AppColors.mutedStrong,
                                ),
                              ),
                            ),
                          ],
                          const SizedBox(height: AppDimens.spaceXl),
                          NeobrutalistButton(
                            label: _isSubmitting
                                ? 'Agendando...'
                                : 'Agendar reunión',
                            icon: Icons.calendar_month_rounded,
                            variant: NeobrutalistButtonVariant.accent,
                            expand: true,
                            onPressed: _isSubmitting ? null : _submit,
                          ),
                          if (_isRealGroup) ...[
                            const SizedBox(height: AppDimens.spaceXl),
                            const AppFieldLabel('PRÓXIMAS REUNIONES'),
                            const SizedBox(height: AppDimens.spaceSm),
                            if (_loadingUpcoming)
                              const Center(
                                child: SizedBox(
                                  width: 22,
                                  height: 22,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                ),
                              )
                            else if (_upcoming.isEmpty)
                              const Text(
                                'Aún no hay reuniones agendadas.',
                                style: TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w600,
                                  color: AppColors.mutedStrong,
                                ),
                              )
                            else
                              for (final m in _upcoming)
                                Container(
                                  margin: const EdgeInsets.only(bottom: 8),
                                  padding: const EdgeInsets.all(12),
                                  decoration: BoxDecoration(
                                    color: AppColors.bg,
                                    border: Border.all(
                                      color: AppColors.border,
                                      width: AppDimens.borderWidth,
                                    ),
                                    borderRadius: BorderRadius.circular(
                                      AppDimens.radius,
                                    ),
                                  ),
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        m.title,
                                        style: const TextStyle(
                                          fontSize: 13,
                                          fontWeight: FontWeight.w800,
                                          color: AppColors.text,
                                        ),
                                      ),
                                      const SizedBox(height: 2),
                                      Text(
                                        _formatWhen(m.scheduledAt),
                                        style: const TextStyle(
                                          fontSize: 11.5,
                                          fontWeight: FontWeight.w700,
                                          color: AppColors.mutedStrong,
                                        ),
                                      ),
                                      if (m.description != null &&
                                          m.description!.trim().isNotEmpty) ...[
                                        const SizedBox(height: 4),
                                        Text(
                                          m.description!.trim(),
                                          maxLines: 3,
                                          overflow: TextOverflow.ellipsis,
                                          style: const TextStyle(
                                            fontSize: 12,
                                            fontWeight: FontWeight.w600,
                                            color: AppColors.text,
                                            height: 1.35,
                                          ),
                                        ),
                                      ],
                                    ],
                                  ),
                                ),
                          ],
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildHeader(bool isDesktop) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'AGENDAR REUNIÓN',
          style: TextStyle(
            fontSize: isDesktop ? 24 : 20,
            fontWeight: FontWeight.w900,
            color: AppColors.text,
            letterSpacing: -0.5,
          ),
        ),
        const SizedBox(height: AppDimens.spaceXs),
        const Text(
          'Elige fecha y hora para reunir al grupo.',
          style: TextStyle(
            fontSize: 12.5,
            fontWeight: FontWeight.w700,
            color: AppColors.mutedStrong,
          ),
        ),
      ],
    );
  }

  Widget _buildPickerBox({
    required String label,
    required IconData icon,
    required String value,
    required VoidCallback onTap,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        AppFieldLabel(label),
        const SizedBox(height: AppDimens.spaceSm),
        ClickCursor(
          child: GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: onTap,
            child: Container(
              padding: const EdgeInsets.symmetric(
                horizontal: AppDimens.spaceMd,
                vertical: 13,
              ),
              decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                borderRadius: BorderRadius.circular(AppDimens.radius),
                boxShadow: AppShadows.badge,
              ),
              child: Row(
                children: [
                  Icon(icon, size: 17, color: AppColors.text),
                  const SizedBox(width: AppDimens.spaceSm),
                  Expanded(
                    child: Text(
                      value,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w900,
                        fontSize: 13,
                        color: AppColors.text,
                      ),
                    ),
                  ),
                  const Icon(
                    Icons.expand_more_rounded,
                    size: 16,
                    color: AppColors.text,
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}

/// Sombra dura bajo los campos de texto: borde de tinta sobre papel.
class _FieldShell extends StatelessWidget {
  const _FieldShell({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(boxShadow: AppShadows.badge),
      child: child,
    );
  }
}
