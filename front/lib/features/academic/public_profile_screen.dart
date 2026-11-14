import 'package:flutter/material.dart';

import '../../core/models/profile_models.dart';
import '../../core/services/profile_service.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';
import '../notes/note_interaction_bar.dart';
import '../notes/notes_api.dart';
import '../notes/public_note_preview_screen.dart';

/// Abre en pantalla completa el perfil público identificado por [rawPublicId].
/// No navega si el ID no tiene el formato válido.
Future<void> openPublicProfile(
  BuildContext context,
  String rawPublicId, {
  VoidCallback? onNoteCopied,
}) {
  final publicId = normalizePublicId(rawPublicId);
  if (publicId == null) return Future<void>.value();
  return Navigator.of(context).push(
    MaterialPageRoute<void>(
      builder: (_) =>
          PublicProfileScreen(publicId: publicId, onNoteCopied: onNoteCopied),
    ),
  );
}

enum _ProfileLoadStatus { loading, ready, notFound, unavailable, failed }

/// Perfil de otro usuario identificado por `public_id` (nunca por UUID):
/// muestra solo su nombre e institución y la lista de sus apuntes públicos.
///
/// Un perfil privado oculta sus datos (Auth responde 404), pero sus apuntes
/// públicos siguen visibles: la pantalla los consulta igualmente y solo
/// muestra «perfil no disponible» si Notes tampoco reconoce el ID.
class PublicProfileScreen extends StatefulWidget {
  const PublicProfileScreen({
    super.key,
    required this.publicId,
    this.profileService,
    this.notesApi,
    this.onNoteCopied,
  });

  final String publicId;
  final ProfileService? profileService;
  final NotesApi? notesApi;
  final VoidCallback? onNoteCopied;

  @override
  State<PublicProfileScreen> createState() => _PublicProfileScreenState();
}

class _PublicProfileScreenState extends State<PublicProfileScreen> {
  late final String? _publicId = normalizePublicId(widget.publicId);
  late final ProfileService _profiles;
  late final NotesApi _notes;

  _ProfileLoadStatus _status = _ProfileLoadStatus.loading;
  PublicProfile? _profile;
  String? _profileError;

  List<PublicProfileNote> _items = const [];
  String _cursor = '';
  bool _notesNotFound = false;
  bool _notesLoading = false;
  bool _loadingMore = false;
  String? _notesError;

  @override
  void initState() {
    super.initState();
    _profiles = widget.profileService ?? ProfileService();
    _notes = widget.notesApi ?? NotesApi();
    _load();
  }

  @override
  void dispose() {
    if (widget.profileService == null) _profiles.dispose();
    if (widget.notesApi == null) _notes.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    if (_publicId == null) {
      setState(() {
        _status = _ProfileLoadStatus.notFound;
        _profileError = 'El ID de perfil no es válido.';
      });
      return;
    }
    setState(() {
      _status = _ProfileLoadStatus.loading;
      _profileError = null;
      _profile = null;
      _items = const [];
      _cursor = '';
      _notesNotFound = false;
      _notesError = null;
      _notesLoading = false;
      _loadingMore = false;
    });
    await _loadProfile();
    // Un perfil privado (404 en Auth) conserva sus apuntes públicos.
    if (mounted &&
        (_status == _ProfileLoadStatus.ready ||
            _status == _ProfileLoadStatus.notFound)) {
      await _loadNotes();
    }
  }

  Future<void> _loadProfile() async {
    try {
      final profile = await _profiles.getPublicProfile(_publicId!);
      if (!mounted) return;
      setState(() {
        _profile = profile;
        _status = _ProfileLoadStatus.ready;
      });
    } on ProfileApiException catch (e) {
      if (!mounted) return;
      final code = e.statusCode ?? 0;
      setState(() {
        if (code == 404 || e.code == 'invalid_public_id') {
          _status = _ProfileLoadStatus.notFound;
          _profileError = 'El ID no existe o el perfil es privado.';
        } else if (code >= 500) {
          _status = _ProfileLoadStatus.unavailable;
          _profileError =
              'No pudimos consultar el perfil en este momento. Inténtalo más tarde.';
        } else {
          _status = _ProfileLoadStatus.failed;
          _profileError = e.message;
        }
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _status = _ProfileLoadStatus.failed;
        _profileError = 'No se pudo cargar el perfil. Revisa tu conexión.';
      });
    }
  }

  Future<void> _loadNotes({bool more = false}) async {
    if (_publicId == null) return;
    final cursor = more ? _cursor : '';
    setState(() {
      if (more) {
        _loadingMore = true;
      } else {
        _notesLoading = true;
        _notesError = null;
          _notesNotFound = false;
        _items = const [];
        _cursor = '';
      }
    });
    try {
      final page = await _notes.listPublicByPublicId(
        _publicId,
        cursor: cursor.isEmpty ? null : cursor,
      );
      if (!mounted) return;
      setState(() {
        _items = more ? [..._items, ...page.items] : page.items;
        _cursor = page.nextCursor;
      });
    } on NotesApiException catch (e) {
      if (!mounted) return;
      final code = e.statusCode ?? 0;
      setState(() {
        if (code == 404) {
          _notesNotFound = true;
          _notesError = 'Este perfil ya no está disponible.';
        } else if (code >= 500) {
          _notesError =
              'No pudimos cargar los apuntes del perfil en este momento.';
        } else {
          _notesError = e.message;
        }
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        _notesError = 'No se pudieron cargar los apuntes del perfil.';
      });
    } finally {
      if (mounted) {
        setState(() {
          _notesLoading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _updateNote(PublicProfileNote note, NoteInteractions state) {
    setState(() {
      _items = [
        for (final item in _items)
          if (item.id == note.id) item.copyWith(interactions: state) else item,
      ];
    });
  }

  Future<void> _openNote(PublicProfileNote note) async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => PublicNotePreviewScreen(
          noteId: note.id,
          initialTitle: note.title,
          api: _notes,
          onNoteCopied: widget.onNoteCopied,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop = context.isDesktop;
    return Scaffold(
      backgroundColor: AppColors.bg,
      body: SafeArea(
        child: MaxWidthContainer(
          maxWidth: 960,
          padding: EdgeInsets.zero,
          child: ListView(
            padding: EdgeInsets.symmetric(
              horizontal: isDesktop ? AppDimens.spaceXl : AppDimens.spaceLg,
              vertical: AppDimens.spaceLg,
            ),
            children: <Widget>[
              _buildHeader(),
              const SizedBox(height: AppDimens.spaceLg),
              ..._buildContent(),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildHeader() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: <Widget>[
        NeobrutalistIconButton(
          icon: Icons.arrow_back_rounded,
          tooltip: 'Volver',
          onPressed: () => Navigator.of(context).maybePop(),
        ),
        const SizedBox(width: AppDimens.spaceMd),
        const Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Text(
                'PERFIL PÚBLICO',
                style: TextStyle(
                  fontSize: 22,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                  letterSpacing: -0.5,
                ),
              ),
              SizedBox(height: 2),
              Text(
                'Su nombre, su institución y sus apuntes públicos',
                style: TextStyle(
                  fontSize: 12.5,
                  fontWeight: FontWeight.w700,
                  color: AppColors.muted,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  List<Widget> _buildContent() {
    switch (_status) {
      case _ProfileLoadStatus.loading:
        return <Widget>[_buildLoadingCard()];
      case _ProfileLoadStatus.ready:
        return <Widget>[
          _buildProfileCard(),
          const SizedBox(height: AppDimens.spaceLg),
          _buildNotesSection(),
        ];
      case _ProfileLoadStatus.notFound:
        // Perfil privado: sus datos no son públicos, pero sus apuntes
        // públicos sí. Solo si Notes tampoco reconoce el ID (404) se muestra
        // «perfil no disponible».
        if (_notesLoading) return <Widget>[_buildLoadingCard()];
        if (!_notesNotFound && _publicId != null) {
          return <Widget>[
            _buildStateCard(
              key: const ValueKey('public-profile-hidden'),
              icon: Icons.visibility_off_rounded,
              title: 'PERFIL PRIVADO',
              message:
                  'Los datos de este perfil no son públicos, pero sus apuntes '
                  'públicos siguen visibles.',
              action: NeobrutalistButton(
                label: 'VOLVER',
                icon: Icons.arrow_back_rounded,
                variant: NeobrutalistButtonVariant.secondary,
                onPressed: () => Navigator.of(context).maybePop(),
              ),
            ),
            const SizedBox(height: AppDimens.spaceLg),
            _buildNotesSection(),
          ];
        }
        return <Widget>[
          _buildStateCard(
            key: const ValueKey('public-profile-not-found'),
            icon: Icons.person_off_rounded,
            title: 'PERFIL NO DISPONIBLE',
            message: _profileError ?? 'El ID no existe o el perfil es privado.',
            action: NeobrutalistButton(
              label: 'VOLVER',
              icon: Icons.arrow_back_rounded,
              variant: NeobrutalistButtonVariant.secondary,
              onPressed: () => Navigator.of(context).maybePop(),
            ),
          ),
        ];
      case _ProfileLoadStatus.unavailable:
        return <Widget>[
          _buildStateCard(
            key: const ValueKey('public-profile-unavailable'),
            icon: Icons.cloud_off_rounded,
            title: 'SERVICIO NO DISPONIBLE',
            message:
                _profileError ??
                'No pudimos consultar el perfil en este momento. Inténtalo más tarde.',
            action: NeobrutalistButton(
              label: 'REINTENTAR',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.accent,
              onPressed: _load,
            ),
          ),
        ];
      case _ProfileLoadStatus.failed:
        return <Widget>[
          _buildStateCard(
            key: const ValueKey('public-profile-error'),
            icon: Icons.error_outline_rounded,
            title: 'NO SE PUDO CARGAR EL PERFIL',
            message:
                _profileError ?? 'Revisa tu conexión e inténtalo de nuevo.',
            action: NeobrutalistButton(
              label: 'REINTENTAR',
              icon: Icons.refresh_rounded,
              variant: NeobrutalistButtonVariant.accent,
              onPressed: _load,
            ),
          ),
        ];
    }
  }

  Widget _buildLoadingCard() {
    return NeobrutalistCard(
      child: SizedBox(
        height: 180,
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: const <Widget>[
            CircularProgressIndicator(color: AppColors.border),
            SizedBox(height: AppDimens.spaceMd),
            Text(
              'CARGANDO PERFIL...',
              style: TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w900,
                letterSpacing: 0.6,
                color: AppColors.muted,
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildStateCard({
    required Key key,
    required IconData icon,
    required String title,
    required String message,
    required Widget action,
  }) {
    return NeobrutalistCard(
      key: key,
      child: Column(
        children: <Widget>[
          Icon(icon, size: 48, color: AppColors.text),
          const SizedBox(height: AppDimens.spaceMd),
          Text(
            title,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontWeight: FontWeight.w900,
              fontSize: 16,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Text(
            message,
            textAlign: TextAlign.center,
            style: const TextStyle(
              fontWeight: FontWeight.w600,
              fontSize: 12.5,
              color: AppColors.mutedStrong,
              height: 1.4,
            ),
          ),
          const SizedBox(height: AppDimens.spaceLg),
          action,
        ],
      ),
    );
  }

  Widget _buildProfileCard() {
    final profile = _profile!;
    final name = profile.displayName.trim().isEmpty
        ? 'Sin nombre'
        : profile.displayName.trim();
    final institution =
        (profile.institution == null || profile.institution!.trim().isEmpty)
        ? 'No informada'
        : profile.institution!.trim();

    return NeobrutalistCard(
      key: const ValueKey('public-profile-card'),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: <Widget>[
          _PublicAvatar(name: name),
          const SizedBox(width: AppDimens.spaceLg),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  name,
                  key: const ValueKey('public-profile-name'),
                  style: const TextStyle(
                    fontSize: 20,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                    letterSpacing: -0.3,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceSm),
                Row(
                  children: <Widget>[
                    const Icon(
                      Icons.apartment_rounded,
                      size: 16,
                      color: AppColors.text,
                    ),
                    const SizedBox(width: AppDimens.spaceSm),
                    Expanded(
                      child: Text(
                        institution,
                        key: const ValueKey('public-profile-institution'),
                        style: const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w700,
                          color: AppColors.mutedStrong,
                        ),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildNotesSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        Row(
          children: <Widget>[
            const Expanded(
              child: Text(
                'APUNTES PÚBLICOS',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w900,
                  color: AppColors.text,
                  letterSpacing: 0.3,
                ),
              ),
            ),
            if (_items.isNotEmpty)
              NeobrutalistBadge(
                label: '${_items.length}',
                icon: Icons.description_rounded,
                compact: true,
              ),
          ],
        ),
        const SizedBox(height: AppDimens.spaceMd),
        if (_notesLoading)
          const Padding(
            padding: EdgeInsets.all(AppDimens.spaceXl),
            child: Center(
              child: CircularProgressIndicator(color: AppColors.border),
            ),
          )
        else if (_notesError != null)
          NeobrutalistCard(
            key: const ValueKey('public-profile-notes-error'),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: <Widget>[
                Text(
                  _notesError!,
                  style: const TextStyle(
                    fontWeight: FontWeight.w800,
                    color: AppColors.text,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceMd),
                NeobrutalistButton(
                  label: 'REINTENTAR',
                  icon: Icons.refresh_rounded,
                  variant: NeobrutalistButtonVariant.accent,
                  onPressed: () => _loadNotes(),
                ),
              ],
            ),
          )
        else if (_items.isEmpty)
          const NeobrutalistCard(
            key: ValueKey('public-profile-notes-empty'),
            child: Text(
              'Este perfil aún no tiene apuntes públicos.',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: AppColors.text,
              ),
            ),
          )
        else ...<Widget>[
          for (final note in _items)
            Padding(
              padding: const EdgeInsets.only(bottom: AppDimens.spaceMd),
              child: _buildNoteCard(note),
            ),
          if (_cursor.isNotEmpty)
            Center(
              child: NeobrutalistButton(
                key: const ValueKey('public-profile-load-more'),
                label: _loadingMore ? 'CARGANDO...' : 'CARGAR MÁS',
                icon: Icons.expand_more_rounded,
                variant: NeobrutalistButtonVariant.secondary,
                onPressed: _loadingMore ? null : () => _loadNotes(more: true),
              ),
            ),
        ],
      ],
    );
  }

  Widget _buildNoteCard(PublicProfileNote note) {
    final when = note.updatedAt ?? note.createdAt;
    return NeobrutalistCard(
      key: ValueKey('public-profile-note-${note.id}'),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Text(
            note.title,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w900,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: AppDimens.spaceSm),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            children: <Widget>[
              const NeobrutalistBadge(
                label: 'Público',
                tone: NeobrutalistTone.success,
                compact: true,
              ),
              if (when != null)
                NeobrutalistBadge(
                  label:
                      '${when.day.toString().padLeft(2, '0')}/'
                      '${when.month.toString().padLeft(2, '0')}/${when.year}',
                  compact: true,
                ),
            ],
          ),
          const SizedBox(height: AppDimens.spaceMd),
          Wrap(
            spacing: AppDimens.spaceSm,
            runSpacing: AppDimens.spaceSm,
            crossAxisAlignment: WrapCrossAlignment.center,
            children: <Widget>[
              NoteLikeSaveBar(
                api: _notes,
                initial: note.interactions,
                compact: true,
                onChanged: (state) => _updateNote(note, state),
              ),
              NeobrutalistButton(
                key: ValueKey('public-profile-open-note-${note.id}'),
                label: 'VER APUNTE',
                icon: Icons.open_in_new_rounded,
                variant: NeobrutalistButtonVariant.info,
                padding: const EdgeInsets.symmetric(
                  horizontal: AppDimens.spaceSm,
                  vertical: AppDimens.spaceXs + 2,
                ),
                onPressed: () => _openNote(note),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

/// Avatar con las iniciales del nombre (la foto del perfil no es pública).
class _PublicAvatar extends StatelessWidget {
  const _PublicAvatar({required this.name});

  final String name;

  @override
  Widget build(BuildContext context) {
    const size = 72.0;
    final initials = name
        .trim()
        .split(RegExp(r'\s+'))
        .where((part) => part.isNotEmpty)
        .take(2)
        .map((part) => part[0].toUpperCase())
        .join();
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radiusSoft),
        boxShadow: AppShadows.badge,
      ),
      child: Text(
        initials.isEmpty ? 'U' : initials,
        style: const TextStyle(
          fontSize: 26,
          fontWeight: FontWeight.w900,
          color: AppColors.text,
        ),
      ),
    );
  }
}
