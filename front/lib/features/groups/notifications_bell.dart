import 'package:flutter/material.dart';

import '../../core/models/social_models.dart';
import '../../core/services/social_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

/// Campanita de notificaciones para el header de grupos (tarea 3_5_14).
/// Muestra badge con no-leídas y abre el diálogo; al cerrar, recarga el conteo.
class NotificationsBell extends StatefulWidget {
  const NotificationsBell({super.key, SocialService? service})
    : _serviceOverride = service;

  final SocialService? _serviceOverride;

  @override
  State<NotificationsBell> createState() => _NotificationsBellState();
}

class _NotificationsBellState extends State<NotificationsBell> {
  late final SocialService _service;
  // Contador compartido con el popup abierto: al marcar una como leída, el
  // badge "N nuevas" del popup y el de la campana bajan a la vez.
  final ValueNotifier<int> _unread = ValueNotifier<int>(0);
  // Un solo popup a la vez: tocar la campana dos veces apilaba overlays
  // duplicados (y cada uno lanzaba su propia carga de notificaciones).
  OverlayEntry? _entry;
  // Descarta respuestas de cargas anteriores que lleguen tarde.
  int _refreshSeq = 0;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _unread.addListener(_onUnreadChanged);
    _refresh();
  }

  void _onUnreadChanged() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    // Si la campana se desmonta con el popup abierto (navegación), el overlay
    // quedaba huérfano usando un SocialService ya cerrado.
    _removeEntry();
    _unread.removeListener(_onUnreadChanged);
    _unread.dispose();
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  void _removeEntry() {
    final entry = _entry;
    _entry = null;
    if (entry != null && entry.mounted) entry.remove();
  }

  Future<void> _refresh() async {
    final seq = ++_refreshSeq;
    try {
      final items = await _service.myNotifications();
      if (!mounted || seq != _refreshSeq) return;
      _unread.value = _countUnread(items);
    } catch (_) {
      // Silencioso: sin sesión o backend caído, la campana no estorba.
    }
  }

  Future<void> _open() async {
    if (_entry != null) return;
    final box = context.findRenderObject() as RenderBox?;
    final overlay = Overlay.of(context);
    final position = box?.localToGlobal(Offset.zero) ?? Offset.zero;
    final size = box?.size ?? const Size(48, 48);

    void close() {
      if (_entry == null) return;
      _removeEntry();
      if (mounted) _refresh();
    }

    final entry = OverlayEntry(
      builder: (_) => _NotificationsPopup(
        service: _service,
        anchorTop: position.dy + size.height + 4,
        unread: _unread,
        onClose: close,
      ),
    );
    _entry = entry;
    overlay.insert(entry);
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      clipBehavior: Clip.none,
      children: [
        IconButton(
          icon: const Icon(Icons.notifications_rounded),
          tooltip: 'Notificaciones',
          onPressed: _open,
        ),
        if (_unread.value > 0)
          Positioned(
            right: 6,
            top: 6,
            // El badge se dibuja encima del botón: sin IgnorePointer capturaba
            // el toque y, con notificaciones sin leer, la campana no abría.
            child: IgnorePointer(
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                decoration: BoxDecoration(
                  color: AppColors.error,
                  border: Border.all(color: AppColors.border, width: 2),
                  borderRadius: BorderRadius.circular(AppDimens.radiusChip),
                ),
                child: Text(
                  _unread.value > 9 ? '9+' : '${_unread.value}',
                  style: const TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.w900,
                    color: AppColors.surface,
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

/// Popup anclado bajo la campana (tarea 3_5_14). Se cierra tocando fuera o
/// con la X; al cerrar, la campana recarga el badge.
class _NotificationsPopup extends StatelessWidget {
  const _NotificationsPopup({
    required this.service,
    required this.anchorTop,
    required this.unread,
    required this.onClose,
  });

  final SocialService service;
  final double anchorTop;
  final ValueNotifier<int> unread;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final popupWidth = (width - 32).clamp(0.0, 340.0);
    return Stack(
      children: [
        GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: onClose,
          child: const SizedBox.expand(),
        ),
        Positioned(
          top: anchorTop,
          right: 12,
          child: Material(
            type: MaterialType.transparency,
            child: Container(
              width: popupWidth,
              constraints: const BoxConstraints(maxHeight: 440),
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
                mainAxisSize: MainAxisSize.min,
                children: [
                  Padding(
                    padding: const EdgeInsets.fromLTRB(14, 10, 4, 10),
                    child: Row(
                      children: [
                        const Expanded(
                          child: Text(
                            'NOTIFICACIONES',
                            style: TextStyle(
                              fontSize: 13,
                              fontWeight: FontWeight.w900,
                              letterSpacing: 0.6,
                              color: AppColors.text,
                              decoration: TextDecoration.none,
                            ),
                          ),
                        ),
                        ValueListenableBuilder<int>(
                          valueListenable: unread,
                          builder: (context, count, _) {
                            if (count <= 0) return const SizedBox.shrink();
                            return Padding(
                              padding: const EdgeInsets.only(
                                right: AppDimens.spaceSm,
                              ),
                              child: NeobrutalistBadge(
                                label: count > 9
                                    ? '9+ nuevas'
                                    : (count == 1
                                          ? '1 nueva'
                                          : '$count nuevas'),
                                tone: NeobrutalistTone.accent,
                                compact: true,
                              ),
                            );
                          },
                        ),
                        NeobrutalistIconButton(
                          icon: Icons.close_rounded,
                          tooltip: 'Cerrar',
                          size: 34,
                          onPressed: onClose,
                        ),
                      ],
                    ),
                  ),
                  Container(height: 2, color: AppColors.border),
                  Flexible(
                    child: _NotificationsList(
                      service: service,
                      onUnreadChanged: (count) => unread.value = count,
                    ),
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

/// Lista de notificaciones; tocar una la marca como leída y recarga.
class _NotificationsList extends StatefulWidget {
  const _NotificationsList({required this.service, this.onUnreadChanged});

  final SocialService service;

  /// Avisa el número de no leídas tras cada carga o marcado.
  final ValueChanged<int>? onUnreadChanged;

  @override
  State<_NotificationsList> createState() => _NotificationsListState();
}

class _NotificationsListState extends State<_NotificationsList> {
  List<MeetingNotification> _items = const [];
  bool _loading = true;
  String? _error;
  // Cargas superpuestas (marcar varias seguidas): solo vale la última.
  int _loadSeq = 0;
  // Ids con POST /read en vuelo: un doble toque no repite la petición.
  final Set<String> _marking = <String>{};

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final seq = ++_loadSeq;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final items = await widget.service.myNotifications();
      if (!mounted || seq != _loadSeq) return;
      final unique = _dedupeById(items);
      setState(() => _items = unique);
      widget.onUnreadChanged?.call(_countUnread(unique));
    } on SocialApiException catch (e) {
      if (!mounted || seq != _loadSeq) return;
      setState(() => _error = e.message);
    } catch (_) {
      // Red caída u otro fallo: antes quedaba como "Sin notificaciones".
      if (!mounted || seq != _loadSeq) return;
      setState(() => _error = 'No se pudieron cargar las notificaciones.');
    } finally {
      if (mounted && seq == _loadSeq) setState(() => _loading = false);
    }
  }

  Future<void> _markRead(MeetingNotification n) async {
    if (n.read || !_marking.add(n.id)) return;
    // Optimista: la fila y los contadores reflejan "leída" al instante; la
    // recarga posterior trae el estado persistido en el backend.
    setState(() {
      _items = [
        for (final item in _items) item.id == n.id ? item.asRead() : item,
      ];
    });
    widget.onUnreadChanged?.call(_countUnread(_items));
    try {
      await widget.service.markNotificationRead(n.id);
    } catch (_) {
      // Best-effort: la recarga muestra el estado real del servidor.
    } finally {
      _marking.remove(n.id);
    }
    if (mounted) await _load();
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Padding(
        padding: EdgeInsets.all(AppDimens.spaceXl),
        child: Center(
          child: SizedBox(
            width: 22,
            height: 22,
            child: CircularProgressIndicator(strokeWidth: 2.5),
          ),
        ),
      );
    }

    if (_error != null) {
      return Padding(
        padding: const EdgeInsets.all(AppDimens.spaceXl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(
              Icons.cloud_off_rounded,
              size: 28,
              color: AppColors.muted,
            ),
            const SizedBox(height: AppDimens.spaceSm),
            Text(
              _error!,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
            const SizedBox(height: AppDimens.spaceMd),
            NeobrutalistButton(
              label: 'Reintentar',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: _load,
            ),
          ],
        ),
      );
    }

    if (_items.isEmpty) {
      return const Padding(
        padding: EdgeInsets.all(AppDimens.spaceXl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.notifications_none_rounded,
              size: 28,
              color: AppColors.muted,
            ),
            SizedBox(height: AppDimens.spaceSm),
            Text(
              'SIN NOTIFICACIONES',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.4,
                color: AppColors.text,
              ),
            ),
            SizedBox(height: AppDimens.spaceXs),
            Text(
              'Aquí verás los recordatorios de tus reuniones.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 11.5,
                fontWeight: FontWeight.w700,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
        ),
      );
    }

    return ListView.builder(
      shrinkWrap: true,
      padding: const EdgeInsets.all(AppDimens.spaceSm),
      itemCount: _items.length,
      itemBuilder: (context, i) => _NotificationTile(
        notification: _items[i],
        onTap: () => _markRead(_items[i]),
      ),
    );
  }
}

/// Fila de notificación: badge Nueva/Leída, sombra dura si no leída y
/// texto multilínea sin overflow.
class _NotificationTile extends StatelessWidget {
  const _NotificationTile({required this.notification, required this.onTap});

  final MeetingNotification notification;
  final VoidCallback onTap;

  static String _formatWhen(DateTime when) {
    final local = when.toLocal();
    return '${local.day.toString().padLeft(2, '0')}/${local.month.toString().padLeft(2, '0')}/${local.year} · '
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final unread = !notification.read;

    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceSm),
      child: Material(
        type: MaterialType.transparency,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(AppDimens.radius),
          child: Container(
            padding: const EdgeInsets.all(AppDimens.spaceMd),
            decoration: BoxDecoration(
              color: unread ? AppColors.surface : AppColors.surfaceLow,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: unread ? const [AppShadows.hard2] : null,
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Icon(
                  unread ? Icons.event_rounded : Icons.event_available_rounded,
                  size: 20,
                  color: AppColors.text,
                ),
                const SizedBox(width: AppDimens.spaceSm),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        notification.title,
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 13,
                          fontWeight: unread
                              ? FontWeight.w900
                              : FontWeight.w700,
                          color: AppColors.text,
                          height: 1.25,
                        ),
                      ),
                      const SizedBox(height: AppDimens.spaceXs),
                      Text(
                        _formatWhen(notification.scheduledAt),
                        style: const TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w700,
                          color: AppColors.mutedStrong,
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: AppDimens.spaceSm),
                NeobrutalistBadge(
                  label: unread ? 'Nueva' : 'Leída',
                  tone: unread
                      ? NeobrutalistTone.accent
                      : NeobrutalistTone.neutral,
                  compact: true,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Número de notificaciones no leídas.
int _countUnread(List<MeetingNotification> items) =>
    items.where((e) => !e.read).length;

/// Una notificación por id aunque la respuesta la repita.
List<MeetingNotification> _dedupeById(List<MeetingNotification> items) {
  final seen = <String>{};
  return [
    for (final n in items)
      if (seen.add(n.id)) n,
  ];
}

extension on MeetingNotification {
  /// Copia marcada como leída (actualización optimista).
  MeetingNotification asRead() => MeetingNotification(
    id: id,
    meetingId: meetingId,
    groupId: groupId,
    title: title,
    scheduledAt: scheduledAt,
    read: true,
    readAt: readAt ?? DateTime.now(),
    createdAt: createdAt,
  );
}
