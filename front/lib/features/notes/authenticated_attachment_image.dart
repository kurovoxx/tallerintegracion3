import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';

/// Private bytes stay in this widget's lifetime; no global image URL cache.
class AuthenticatedAttachmentImage extends StatefulWidget {
  const AuthenticatedAttachmentImage({
    super.key,
    required this.contentUri,
    required this.fit,
    required this.onOpenDrive,
    this.client,
    this.debugLabel = 'ATTACHMENT',
    this.externalFileId,
    this.localBytes,
  });
  final Uri contentUri;
  final BoxFit fit;
  final VoidCallback onOpenDrive;
  final http.Client? client;
  final String debugLabel;

  /// `external_file_id` de Drive del adjunto. Si es nulo o vacío, la nota es
  /// local y el binario todavía no vive en Drive: se sirve [localBytes] en
  /// lugar del endpoint remoto.
  final String? externalFileId;

  /// Bytes locales del adjunto (en memoria) para previsualizarlo cuando la nota
  /// aún no se ha sincronizado, sin disparar "Imagen no disponible".
  final Uint8List? localBytes;

  @override
  State<AuthenticatedAttachmentImage> createState() =>
      _AuthenticatedAttachmentImageState();
}

class _AuthenticatedAttachmentImageState
    extends State<AuthenticatedAttachmentImage> {
  late Future<Uint8List> _bytes;
  @override
  void initState() {
    super.initState();
    _bytes = _load();
  }

  @override
  void didUpdateWidget(covariant AuthenticatedAttachmentImage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.contentUri != widget.contentUri ||
        oldWidget.client != widget.client ||
        oldWidget.externalFileId != widget.externalFileId ||
        oldWidget.localBytes != widget.localBytes) {
      _bytes = _load();
    }
  }

  String get _attachmentId {
    final segs = widget.contentUri.pathSegments;
    final idx = segs.indexOf('attachments');
    if (idx >= 0 && idx + 1 < segs.length) return segs[idx + 1];
    return segs.isNotEmpty ? segs.last : '';
  }

  Future<Uint8List> _load() async {
    final local = widget.localBytes;
    final external = widget.externalFileId?.trim() ?? '';
    // Nota local todavía sin external_file_id: el binario ya está en memoria y
    // no tiene sentido pedir un endpoint que aún no lo puede servir.
    if (local != null && local.isNotEmpty && external.isEmpty) {
      debugPrint(
        '[Notes] ${widget.debugLabel} source=local '
        'external_file_id=(vacío) attachment=$_attachmentId',
      );
      return local;
    }
    final client = widget.client ?? http.Client();
    try {
      final response = await AuthedHttp.run(() async {
        final token = SessionManager.token;
        if (token == null || token.isEmpty) throw StateError('Sin sesión');
        return client
            .get(widget.contentUri, headers: {'Authorization': 'Bearer $token'})
            .timeout(const Duration(seconds: 30));
      });
      final mime = response.headers['content-type'] ?? '';
      debugPrint(
        '[Notes] ${widget.debugLabel} path=${widget.contentUri.path} '
        'attachment=$_attachmentId status=${response.statusCode} mime=$mime',
      );
      if (response.statusCode != 200 ||
          !mime.startsWith('image/') ||
          response.bodyBytes.length > 10 * 1024 * 1024) {
        if (local != null && local.isNotEmpty) return local;
        throw StateError('Imagen no disponible');
      }
      return response.bodyBytes;
    } catch (e) {
      // Endpoint no disponible (404/403/offline): si hay copia local se sirve
      // en su lugar; solo entonces se muestra el error de imagen.
      if (local != null && local.isNotEmpty) {
        debugPrint(
          '[Notes] ${widget.debugLabel} attachment=$_attachmentId '
          'fallback=local error=$e',
        );
        return local;
      }
      rethrow;
    } finally {
      if (widget.client == null) client.close();
    }
  }

  Widget _error() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Text(
          'Imagen no disponible',
          textAlign: TextAlign.center,
          style: TextStyle(
            color: AppColors.text,
            fontWeight: FontWeight.w700,
            fontSize: 12,
          ),
        ),
        TextButton(
          onPressed: widget.onOpenDrive,
          child: const Text('Abrir en Drive'),
        ),
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => FutureBuilder<Uint8List>(
    future: _bytes,
    builder: (context, snapshot) {
      if (snapshot.connectionState != ConnectionState.done) {
        return const Center(
          child: SizedBox(
            width: 20,
            height: 20,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: AppColors.border,
            ),
          ),
        );
      }
      if (snapshot.hasError || !snapshot.hasData) return _error();
      // SizedBox.expand garantiza que el binario ocupe todo el contenedor y que
      // BoxFit.cover/contain no deje franjas transparentes o blancas visibles.
      return SizedBox.expand(
        child: Image.memory(
          snapshot.data!,
          fit: widget.fit,
          gaplessPlayback: true,
          errorBuilder: (_, _, _) => _error(),
        ),
      );
    },
  );
}
