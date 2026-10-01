// Ruta: front/lib/features/workspace/schedule_meeting_screen.dart
//
// Formulario "Agendar reunión": ficha neobrutalista retro con tarjeta de
// borde 2.5 px y sombra hard4, campos de tinta, selectores de fecha/hora
// tipo sticker y chips invertidos para los integrantes. Consume los tokens
// canónicos de core/theme/app_theme.dart y core/widgets/neobrutalism.dart.

import 'package:flutter/material.dart';

import '../../core/common_widgets.dart';
import '../../core/models/social_models.dart';
import '../../core/services/api_config.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../auth/google_calendar_service.dart';
import '../auth/google_drive_service.dart' show LocalPortBusyException;

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
  final _emailCtrl = TextEditingController();
  final Map<String, String> _guests = {};
  String? _guestError;
  final _linkCtrl = TextEditingController();
  List<GroupMember> _members = const [];
  bool _loadingMembers = false;
  String? _membersError;
  DateTime _selectedDate = DateTime.now().add(const Duration(days: 1));
  TimeOfDay _selectedTime = const TimeOfDay(hour: 10, minute: 0);

  late final SocialService _social;
  final GoogleCalendarService _calendar = GoogleCalendarService();
  bool _isSubmitting = false;
  String? _createdMeetingId;
  String? _submitError;
  List<MeetingItem> _upcoming = const [];
  bool _loadingUpcoming = false;
  CalendarStatus? _calStatus;
  bool _calLoading = false;
  bool _calConnecting = false;
  String? _calError;

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? SocialService();
    if (_isRealGroup) {
      _loadUpcoming();
      _loadMembers();
    }
    _loadCalendarStatus();
  }

  @override
  void dispose() {
    _titleCtrl.dispose();
    _descCtrl.dispose();
    _linkCtrl.dispose();
    _emailCtrl.dispose();
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

  Future<void> _loadMembers() async {
    setState(() => _loadingMembers = true);
    try {
      final members = await _social.listMembers(widget.groupId!.trim());
      if (!mounted) return;
      setState(() => _members = members);
    } catch (_) {
      if (!mounted) return;
      setState(() => _membersError = 'No se pudieron cargar los integrantes.');
    } finally {
      if (mounted) setState(() => _loadingMembers = false);
    }
  }

  String _memberLabel(GroupMember member) {
    if (member.displayName?.trim().isNotEmpty == true) {
      return member.displayName!.trim();
    }
    if (member.email?.trim().isNotEmpty == true) return member.email!.trim();
    return 'Integrante';
  }

  void _addGuest(String raw, [String? label]) {
    try {
      final email = raw.trim().toLowerCase();
      if (email.isEmpty) {
        throw const FormatException('Ingresa un correo válido.');
      }
      normalizeMeetingAttendees([..._guests.keys, email]);
      setState(() {
        _guests[email] = label ?? email;
        _guestError = null;
        _emailCtrl.clear();
      });
    } on FormatException catch (e) {
      setState(() => _guestError = e.message);
    }
  }

  /// Estado real de Calendar para la cuenta activa (aislado por JWT,
  /// igual que Drive). null = sin red/sesión: se conserva el estado previo.
  Future<void> _loadCalendarStatus() async {
    if (!mounted) return;
    setState(() {
      _calLoading = true;
      _calError = null;
    });
    try {
      await GoogleCalendarService.ensureConfigured(
        backendBaseUrl: authApiBaseUrl,
      );
      final status = await _calendar.fetchStatus(
        backendBaseUrl: authApiBaseUrl,
      );
      if (!mounted) return;
      setState(() {
        if (status != null) _calStatus = status;
        _calLoading = false;
      });
    } catch (_) {
      if (mounted) setState(() => _calLoading = false);
    }
  }

  /// Flujo OAuth real: navegador → code → POST connect → status.
  /// Nunca guarda tokens de Google en el frontend.
  Future<void> _linkGoogleCalendar() async {
    if (_calConnecting) return;
    setState(() {
      _calConnecting = true;
      _calError = null;
    });
    try {
      await GoogleCalendarService.ensureConfigured(
        backendBaseUrl: authApiBaseUrl,
      );
      final auth = await _calendar.getCalendarAuthCode();
      if (!mounted) return;
      if (auth == null) {
        setState(() {
          _calConnecting = false;
          _calError = 'Autorización cancelada. Vuelve a intentarlo si quieres vincular Calendar.';
        });
        return;
      }
      final result = await _calendar.connectCalendar(
        backendBaseUrl: authApiBaseUrl,
        oauthCode: auth.code,
        redirectUri: auth.redirectUri.isEmpty ? null : auth.redirectUri,
      );
      if (!mounted) return;
      if (!result.ok) {
        setState(() {
          _calConnecting = false;
          _calError = result.message;
        });
        return;
      }
      final status = await _calendar.fetchStatus(
        backendBaseUrl: authApiBaseUrl,
      );
      if (!mounted) return;
      setState(() {
        _calConnecting = false;
        if (status != null) _calStatus = status;
      });
      if (mounted && (status?.connected ?? false)) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Google Calendar vinculado.')),
        );
      }
    } on LocalPortBusyException catch (e) {
      if (mounted) {
        setState(() {
          _calConnecting = false;
          _calError = e.toString();
        });
      }
    } on StateError catch (e) {
      if (mounted) {
        setState(() {
          _calConnecting = false;
          _calError = e.message;
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          _calConnecting = false;
          _calError =
              'No se pudo conectar con Google Calendar. Inténtalo nuevamente más tarde.';
        });
      }
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
  // Invitados por email; enlace/sala continúa como dato local del formulario.
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
        attendees: _guests.keys.toList(),
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
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(
        () => _submitError = e.message.contains('invalid attendee email')
            ? 'Ingresa un correo válido.'
            : e.message.contains('too many attendees')
            ? 'Puedes invitar hasta 50 personas.'
            : 'No se pudo agendar la reunión.',
      );
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
                          const AppFieldLabel('ENLACE / SALA'),
                          const SizedBox(height: AppDimens.spaceSm),
                          _FieldShell(
                            child: TextFormField(
                              controller: _linkCtrl,
                              decoration: appInputDecoration(
                                'https://meet.google.com/...',
                              ),
                              style: const TextStyle(
                                fontWeight: FontWeight.w700,
                                fontSize: 13,
                                color: AppColors.text,
                              ),
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('GOOGLE CALENDAR'),
                          const SizedBox(height: AppDimens.spaceSm),
                          if (_calLoading)
                            const LinearProgressIndicator()
                          else if (_calStatus?.connected == true) ...[
                            Container(
                              padding: const EdgeInsets.all(
                                AppDimens.spaceMd,
                              ),
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
                              child: Row(
                                children: [
                                  const Icon(
                                    Icons.check_circle_rounded,
                                    color: AppColors.text,
                                    size: 18,
                                  ),
                                  const SizedBox(width: 8),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        const Text(
                                          '✓ Google Calendar vinculado',
                                          style: TextStyle(
                                            fontSize: 12,
                                            fontWeight: FontWeight.w900,
                                            color: AppColors.text,
                                          ),
                                        ),
                                        if (_calStatus?.externalEmail !=
                                            null)
                                          Text(
                                            _calStatus!.externalEmail!,
                                            style: const TextStyle(
                                              fontSize: 11,
                                              fontWeight: FontWeight.w600,
                                              color: AppColors.muted,
                                            ),
                                          ),
                                      ],
                                    ),
                                  ),
                                ],
                              ),
                            ),
                            const SizedBox(height: AppDimens.spaceSm),
                            NeobrutalistButton(
                              label: _calConnecting
                                  ? 'CONECTANDO...'
                                  : 'RECONECTAR',
                              icon: Icons.refresh_rounded,
                              variant:
                                  NeobrutalistButtonVariant.secondary,
                              expand: true,
                              onPressed: _calConnecting
                                  ? null
                                  : _linkGoogleCalendar,
                            ),
                          ] else ...[
                            NeobrutalistButton(
                              label: _calConnecting
                                  ? 'CONECTANDO...'
                                  : 'VINCULAR CON GOOGLE CALENDAR',
                              icon: Icons.calendar_today_rounded,
                              variant: NeobrutalistButtonVariant.info,
                              expand: true,
                              borderWidth: AppDimens.borderWidthAction,
                              onPressed: _calConnecting
                                  ? null
                                  : _linkGoogleCalendar,
                            ),
                          ],
                          if (_calError != null) ...[
                            const SizedBox(height: AppDimens.spaceSm),
                            Text(
                              _calError!,
                              style: const TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.w700,
                                color: AppColors.error,
                              ),
                            ),
                          ],
                          const SizedBox(height: AppDimens.spaceMd),
                          const AppFieldLabel('INVITAR MIEMBROS DEL GRUPO'),
                          const Text(
                            'Toca un integrante para agregarlo con su correo real. El chip muestra el nombre; internamente se guarda el email.',
                            style: TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w600,
                              color: AppColors.muted,
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceSm),
                          if (_loadingMembers) const LinearProgressIndicator(),
                          if (_membersError != null) Text(_membersError!),
                          if (!_loadingMembers &&
                              _membersError == null &&
                              _members.isEmpty)
                            const Text('No hay integrantes disponibles.'),
                          Wrap(
                            spacing: AppDimens.spaceSm,
                            runSpacing: AppDimens.spaceSm,
                            children: [
                              for (final member in _members)
                                _MemberStickerChip(
                                  label: _memberLabel(member),
                                  selected: _guests.containsKey(
                                    member.email?.trim().toLowerCase(),
                                  ),
                                  onTap: () {
                                    final email =
                                        member.email?.trim().toLowerCase() ??
                                        '';
                                    if (_guests.containsKey(email)) {
                                      setState(() => _guests.remove(email));
                                    } else if (email.isEmpty) {
                                      setState(
                                        () => _guestError =
                                            'Este integrante no tiene correo disponible. Escríbelo abajo para invitarlo.',
                                      );
                                    } else {
                                      _addGuest(email, _memberLabel(member));
                                    }
                                  },
                                ),
                            ],
                          ),
                          const SizedBox(height: AppDimens.spaceXl),
                          const AppFieldLabel(
                            'INVITAR POR CORREO (EXTERNO)',
                          ),
                          const Text(
                            'Solo para alguien fuera del grupo. Si es miembro, selecciónalo arriba.',
                            style: TextStyle(
                              fontSize: 11,
                              fontWeight: FontWeight.w600,
                              color: AppColors.muted,
                            ),
                          ),
                          const SizedBox(height: AppDimens.spaceSm),
                          TextField(
                            controller: _emailCtrl,
                            decoration: InputDecoration(
                              labelText: 'Correo del invitado',
                              errorText: _guestError,
                            ),
                            keyboardType: TextInputType.emailAddress,
                            onSubmitted: (value) => _addGuest(value),
                          ),
                          TextButton(
                            onPressed: () => _addGuest(_emailCtrl.text),
                            child: const Text('Añadir invitado'),
                          ),
                          Wrap(
                            spacing: 8,
                            children: [
                              for (final guest in _guests.entries)
                                InputChip(
                                  label: Text(guest.value),
                                  onDeleted: () =>
                                      setState(() => _guests.remove(guest.key)),
                                ),
                            ],
                          ),
                          const SizedBox(height: 12),
                          NeobrutalistButton(
                            label: _isSubmitting
                                ? 'Agendando...'
                                : 'Agendar reunión',
                            icon: Icons.calendar_month_rounded,
                            variant: NeobrutalistButtonVariant.accent,
                            expand: true,
                            onPressed: _isSubmitting ? null : _submit,
                          ),
                        ],
                      ),
                    ),
                  ),
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
                        borderRadius: BorderRadius.circular(AppDimens.radius),
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
                        borderRadius: BorderRadius.circular(AppDimens.radius),
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
                        borderRadius: BorderRadius.circular(AppDimens.radius),
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
                  if (_isRealGroup) ...[
                    const SizedBox(height: AppDimens.spaceXl),
                    const AppFieldLabel('PRÓXIMAS REUNIONES'),
                    const SizedBox(height: AppDimens.spaceSm),
                    if (_loadingUpcoming)
                      const Center(
                        child: SizedBox(
                          width: 22,
                          height: 22,
                          child: CircularProgressIndicator(strokeWidth: 2),
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
                            crossAxisAlignment: CrossAxisAlignment.start,
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

/// Chip sticker de integrante: borde de 2 px, sombra dura cuando está libre y
/// fondo resaltador (invertido) al seleccionarse.
class _MemberStickerChip extends StatefulWidget {
  const _MemberStickerChip({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  State<_MemberStickerChip> createState() => _MemberStickerChipState();
}

class _MemberStickerChipState extends State<_MemberStickerChip> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final pressed = _pressed;
    final selected = widget.selected;
    return Semantics(
      button: true,
      selected: selected,
      child: MouseRegion(
        cursor: SystemMouseCursors.click,
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
              pressed ? AppShadows.offsetBadge.dx : 0,
              pressed ? AppShadows.offsetBadge.dy : 0,
              0,
            ),
            padding: const EdgeInsets.symmetric(
              horizontal: AppDimens.spaceMd,
              vertical: 7,
            ),
            decoration: BoxDecoration(
              color: selected ? AppColors.accentYellow : AppColors.surface,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              boxShadow: selected || pressed
                  ? const <BoxShadow>[]
                  : AppShadows.badge,
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(
                  selected
                      ? Icons.check_box_rounded
                      : Icons.check_box_outline_blank_rounded,
                  size: 13,
                  color: AppColors.text,
                ),
                const SizedBox(width: AppDimens.spaceXs),
                Text(
                  widget.label.toUpperCase(),
                  style: const TextStyle(
                    fontSize: 11,
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
