import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import '../../core/services/authed_client.dart';
import '../../core/services/session_manager.dart';

/// Private bytes stay in this widget's lifetime; no global image URL cache.
class AuthenticatedAttachmentImage extends StatefulWidget {
  const AuthenticatedAttachmentImage({
    super.key,
    required this.contentUri,
    required this.fit,
    required this.onOpenDrive,
    this.client,
  });
  final Uri contentUri;
  final BoxFit fit;
  final VoidCallback onOpenDrive;
  final http.Client? client;

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
        oldWidget.client != widget.client) {
      _bytes = _load();
    }
  }

  Future<Uint8List> _load() async {
    final client = widget.client ?? http.Client();
    try {
      final response = await AuthedHttp.run(() async {
        final token = SessionManager.token;
        if (token == null || token.isEmpty) throw StateError('Sin sesión');
        return client
            .get(widget.contentUri, headers: {'Authorization': 'Bearer $token'})
            .timeout(const Duration(seconds: 30));
      });
      if (response.statusCode != 200 ||
          !(response.headers['content-type'] ?? '').startsWith('image/') ||
          response.bodyBytes.length > 10 * 1024 * 1024) {
        throw StateError('Imagen no disponible');
      }
      return response.bodyBytes;
    } finally {
      if (widget.client == null) client.close();
    }
  }

  Widget _error() => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        const Text('Imagen no disponible', textAlign: TextAlign.center),
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
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        );
      }
      if (snapshot.hasError || !snapshot.hasData) return _error();
      return Image.memory(
        snapshot.data!,
        fit: widget.fit,
        errorBuilder: (_, _, _) => _error(),
      );
    },
  );
}
