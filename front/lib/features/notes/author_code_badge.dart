import 'package:flutter/material.dart';

import '../../core/widgets/neobrutalism.dart';
import '../academic/public_profile_screen.dart';
import 'notes_api.dart';

/// Insignia «Autor código» de las tarjetas de apuntes guardados y públicos.
///
/// Muestra el código público del autor (nunca el identificador interno). Si
/// el código está disponible, tocarla abre el perfil del autor con su lista
/// de apuntes públicos, el mismo flujo que «Buscar perfil». Sin código
/// muestra solo «Autor» y no es interactiva.
class AuthorCodeBadge extends StatelessWidget {
  const AuthorCodeBadge({
    super.key,
    required this.authorPublicId,
    this.onNoteCopied,
  });

  final String? authorPublicId;

  /// Se reenvía al perfil del autor (p. ej. para refrescar «Mis apuntes»).
  final VoidCallback? onNoteCopied;

  @override
  Widget build(BuildContext context) {
    final code = authorPublicId;
    final badge = NeobrutalistBadge(
      label: authorBadgeLabel(code),
      icon: Icons.person_rounded,
      compact: true,
    );
    if (code == null) return badge;
    return Semantics(
      button: true,
      label: 'Ver perfil del autor $code',
      excludeSemantics: true,
      child: Tooltip(
        message: 'Ver perfil y apuntes del autor',
        child: InkWell(
          key: ValueKey('author-code-$code'),
          onTap: () =>
              openPublicProfile(context, code, onNoteCopied: onNoteCopied),
          child: badge,
        ),
      ),
    );
  }
}
