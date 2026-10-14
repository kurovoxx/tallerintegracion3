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
  int _unread = 0;

  @override
  void initState() {
    super.initState();
    _service = widget._serviceOverride ?? SocialService();
    _refresh();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _service.dispose();
    super.dispose();
  }

  Future<void> _refresh() async {
    try {
      final items = await _service.myNotifications();
      if (!mounted) return;
      setState(() => _unread = items.where((e) => !e.read).length);
    } catch (_) {
      // Silencioso: sin sesión o backend caído, la campana no estorba.
    }
  }

  Future<void> _open() async {
    final box = context.findRenderObject() as RenderBox?;
    final overlay = Overlay.of(context);
    final position = box?.localToGlobal(Offset.zero) ?? Offset.zero;
    final size = box?.size ?? const Size(48, 48);

    late final OverlayEntry entry;
    void close() {
      entry.remove();
      if (mounted) _refresh();
    }

    entry = OverlayEntry(
      builder: (_) => _NotificationsPopup(
        service: _service,
        anchorTop: position.dy + size.height + 4,
        onClose: close,
      ),
    );
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
        if (_unread > 0)
          Positioned(
            right: 6,
            top: 6,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
              decoration: BoxDecoration(
                color: AppColors.error,
                border: Border.all(color: Colors.black, width: 1.5),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Text(
                _unread > 9 ? '9+' : '$_unread',
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: Colors.white,
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
    required this.onClose,
  });

  final SocialService service;
  final double anchorTop;
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
          child: Container(color: Colors.transparent),
        ),
        Positioned(
          top: anchorTop,
          right: 12,
          child: Container(
            width: popupWidth,
            constraints: const BoxConstraints(maxHeight: 420),
            decoration: BoxDecoration(
              color: Colors.white,
              border: Border.all(color: AppColors.border, width: 2),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: AppShadows.card,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(12, 10, 4, 6),
                  child: Row(
                    children: [
                      const Expanded(
                        child: Text(
                          'NOTIFICACIONES',
                          style: TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w900,
                          ),
                        ),
                      ),
                      IconButton(
                        icon: const Icon(Icons.close_rounded, size: 18),
                        tooltip: 'Cerrar',
                        onPressed: onClose,
                      ),
                    ],
                  ),
                ),
                Container(height: 2, color: AppColors.border),
                Flexible(child: _NotificationsList(service: service)),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

/// Lista de notificaciones; tocar una la marca como leída y recarga.
class _NotificationsList extends StatefulWidget {
  const _NotificationsList({required this.service});

  final SocialService service;

  @override
  State<_NotificationsList> createState() => _NotificationsListState();
}

class _NotificationsListState extends State<_NotificationsList> {
  List<MeetingNotification> _items = const [];
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final items = await widget.service.myNotifications();
      if (!mounted) return;
      setState(() => _items = items);
    } on SocialApiException catch (e) {
      if (!mounted) return;
      setState(() => _error = e.message);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _markRead(MeetingNotification n) async {
    if (n.read) return;
    try {
      await widget.service.markNotificationRead(n.id);
    } catch (_) {
      // Best-effort: igual se recarga la lista.
    }
    if (mounted) _load();
  }

  static String _formatWhen(DateTime when) {
    final local = when.toLocal();
    return '${local.day.toString().padLeft(2, '0')}/${local.month.toString().padLeft(2, '0')}/${local.year} · '
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    Widget content;
    if (_loading) {
      content = const Padding(
        padding: EdgeInsets.all(16),
        child: Center(
          child: SizedBox(
            width: 24,
            height: 24,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        ),
      );
    } else if (_error != null) {
      content = Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(_error!),
          const SizedBox(height: 8),
          NeobrutalistButton(
            label: 'Reintentar',
            icon: Icons.refresh_rounded,
            onPressed: _load,
          ),
        ],
      );
    } else if (_items.isEmpty) {
      content = const Padding(
        padding: EdgeInsets.all(12),
        child: Text('Sin notificaciones.'),
      );
    } else {
      content = ListView.builder(
        shrinkWrap: true,
        padding: const EdgeInsets.all(8),
        itemCount: _items.length,
        itemBuilder: (context, i) {
            final n = _items[i];
            return Container(
              margin: const EdgeInsets.only(bottom: 8),
              decoration: BoxDecoration(
                color: n.read ? AppColors.bg : Colors.white,
                border: Border.all(color: AppColors.border, width: 2),
                borderRadius: BorderRadius.circular(AppDimens.radius),
              ),
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => _markRead(n),
                child: Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 10,
                  ),
                  child: Row(
                    children: [
                      Icon(
                        n.read
                            ? Icons.event_available_rounded
                            : Icons.event_rounded,
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Text(
                              n.title,
                              style: TextStyle(
                                fontSize: 13,
                                fontWeight: n.read
                                    ? FontWeight.w600
                                    : FontWeight.w800,
                              ),
                            ),
                            const SizedBox(height: 2),
                            Text(
                              _formatWhen(n.scheduledAt),
                              style: const TextStyle(fontSize: 11),
                            ),
                          ],
                        ),
                      ),
                      if (!n.read) const Icon(Icons.circle, size: 10),
                    ],
                  ),
                ),
              ),
            );
          },
        );
    }
    return content;
  }
}
