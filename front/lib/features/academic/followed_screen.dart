/// Ruta: front/lib/features/academic/followed_screen.dart
///
/// Pestaña "Seguidos": lista las personas que el usuario autenticado sigue.
/// Cada fila muestra el avatar, nombre, correo (si lo tiene) y un botón para
/// dejar de seguir. Al tocar el nombre o avatar se abre el perfil público.
/// Estilo Neo-Brutalism, consistente con GroupMembersTab.

import 'dart:convert';

import 'package:flutter/material.dart';

import '../../core/models/profile_models.dart';
import '../../core/services/profile_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../../core/widgets/user_avatar.dart';
import 'public_profile_screen.dart';

/// Pantalla de seguidos: muestra a quién sigue el usuario actual.
class FollowedScreen extends StatefulWidget {
  const FollowedScreen({super.key, ProfileService? service})
    : _serviceOverride = service;

  final ProfileService? _serviceOverride;

  @override
  State<FollowedScreen> createState() => _FollowedScreenState();
}

class _FollowedScreenState extends State<FollowedScreen> {
  late final ProfileService _social;

  bool _loading = true;
  String? _error;
  List<FollowedUser> _following = const [];
  String? _busyUserId;

  String? get _me => SessionManager.currentUserId;

  @override
  void initState() {
    super.initState();
    _social = widget._serviceOverride ?? ProfileService();
    _load();
  }

  @override
  void dispose() {
    if (widget._serviceOverride == null) _social.dispose();
    super.dispose();
  }

  void _snack(String message, {bool error = false}) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        backgroundColor: error ? AppColors.errorDeep : null,
      ),
    );
  }

  Future<void> _load() async {
    final profile = ProfileService.current.value;
    if (profile == null || profile.publicId.isEmpty) {
      setState(() {
        _loading = false;
        _error = 'No se pudo obtener tu código de perfil.';
        _following = const [];
      });
      return;
    }
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final list = await _social.listFollowing(profile.publicId);
      if (!mounted) return;
      setState(() {
        _following = list;
        _loading = false;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        _error = 'No se pudieron cargar los seguidos.';
      });
    }
  }

  /// Alterna dejar de seguir a un usuario.
  Future<void> _toggleUnfollow(FollowedUser followed) async {
    setState(() => _busyUserId = followed.userId);
    try {
      await _social.unfollowUser(followed.publicId);
      if (!mounted) return;
      _load();
      _snack('Has dejado de seguir a ${followed.displayName}.');
    } catch (_) {
      if (!mounted) return;
      _snack('No se pudo dejar de seguir.', error: true);
    } finally {
      if (mounted) setState(() => _busyUserId = null);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: Column(
          children: <Widget>[
            _header(),
            Divider(
              color: AppColors.border,
              thickness: AppDimens.borderWidth,
              height: 1,
            ),
            Expanded(child: _body()),
          ],
        ),
      ),
    );
  }

  Widget _header() {
    return Container(
      color: AppColors.surface,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceLg,
        vertical: AppDimens.spaceSm,
      ),
      child: Row(
        children: <Widget>[
          Icon(Icons.person_add_rounded, size: 16, color: AppColors.text),
          const SizedBox(width: AppDimens.spaceSm),
          const Expanded(
            child: Text(
              'SIGUIENDO',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 12,
                letterSpacing: 0.5,
              ),
            ),
          ),
          if (_following.isNotEmpty)
            Container(
              padding: const EdgeInsets.symmetric(
                horizontal: 6,
                vertical: 2,
              ),
              decoration: BoxDecoration(
                color: AppColors.accentYellow.withValues(alpha: 0.15),
                border: Border.all(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                borderRadius: BorderRadius.circular(AppDimens.radiusChip),
              ),
              child: Text(
                '${_following.length}',
                style: TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.w900,
                  color: AppColors.accentYellow,
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _body() {
    if (_loading) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(AppDimens.spaceXl),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              const Icon(Icons.error_outline_rounded, size: 32),
              const SizedBox(height: AppDimens.spaceSm),
              Text(_error!, textAlign: TextAlign.center),
              const SizedBox(height: AppDimens.spaceMd),
              NeobrutalistButton(
                label: 'Reintentar',
                variant: NeobrutalistButtonVariant.secondary,
                onPressed: _load,
              ),
            ],
          ),
        ),
      );
    }
    if (_following.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: <Widget>[
            Icon(Icons.person_add_outlined, size: 64, color: AppColors.muted),
            SizedBox(height: AppDimens.spaceMd),
            Text(
              'Aún no sigues a nadie.',
              style: TextStyle(
                fontWeight: FontWeight.w900,
                fontSize: 16,
                color: AppColors.text,
              ),
            ),
            SizedBox(height: AppDimens.spaceSm),
            Text(
              'Busca un perfil por código y dale a «Seguir» para que aparezca aquí.',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                color: AppColors.mutedStrong,
              ),
            ),
          ],
        ),
    );
  }
    return ListView.builder(
      padding: const EdgeInsets.all(AppDimens.spaceMd),
      itemCount: _following.length,
      itemBuilder: (_, i) => _followedCard(_following[i]),
    );
  }

  Widget _followedCard(FollowedUser u) {
    final busy = _busyUserId == u.userId;
    return Padding(
      padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
      child: NeobrutalistCard(
        padding: const EdgeInsets.all(AppDimens.spaceMd),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                _avatar(u),
                const SizedBox(width: AppDimens.spaceMd),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: <Widget>[
                      Text(
                        u.displayName,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          fontWeight: FontWeight.w900,
                          fontSize: 14,
                        ),
                      ),
                      if (u.publicId.isNotEmpty)
                        Text(
                          'Código: ${u.publicId}',
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontSize: 11,
                            fontWeight: FontWeight.w600,
                            color: AppColors.muted,
                          ),
                        ),
                      const SizedBox(height: AppDimens.spaceSm),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: AppDimens.spaceMd),
            Row(
              children: <Widget>[
                _action(
                  'Ver perfil',
                  Icons.person_rounded,
                  NeobrutalistButtonVariant.secondary,
                  () => _openProfile(u),
                ),
                const SizedBox(width: AppDimens.spaceSm),
                _action(
                  'Dejar de seguir',
                  Icons.person_remove_rounded,
                  NeobrutalistButtonVariant.secondary,
                  busy ? null : () => _toggleUnfollow(u),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  ImageProvider<Object>? _photoProvider(FollowedUser u) {
    final url = u.photoThumbUrl;
    if (url == null || url.isEmpty) return null;
    const marker = ';base64,';
    final at = url.indexOf(marker);
    if (at <= 0) {
      // URL HTTP normal
      return NetworkImage(url);
    }
    try {
      return MemoryImage(base64Decode(url.substring(at + marker.length)));
    } on FormatException {
      return null;
    }
  }

  Widget _avatar(FollowedUser u) {
    return CircleAvatar(
      radius: 20,
      backgroundImage: _photoProvider(u),
      child: u.photoThumbUrl == null || u.photoThumbUrl!.isEmpty
          ? Text(
              u.displayName.isNotEmpty ? u.displayName[0].toUpperCase() : '?',
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
              ),
            )
          : null,
    );
  }

  Widget _action(
    String label,
    IconData icon,
    NeobrutalistButtonVariant variant,
    VoidCallback? onPressed,
  ) {
    return NeobrutalistButton(
      label: label,
      icon: icon,
      variant: variant,
      padding: const EdgeInsets.symmetric(
        horizontal: AppDimens.spaceMd,
        vertical: AppDimens.spaceSm,
      ),
      onPressed: onPressed,
    );
  }

  Future<void> _openProfile(FollowedUser u) async {
    if (u.publicId.isEmpty) return;
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PublicProfileScreen(publicId: u.publicId),
      ),
    );
  }
}
