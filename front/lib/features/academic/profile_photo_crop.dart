import 'dart:convert';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

import 'package:file_selector/file_selector.dart';
import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';
import '../../core/widgets/neobrutalism.dart';

const int kProfilePhotoOutputSize = 384;
const double kProfilePhotoMaxZoom = 4;
const String kProfilePhotoDataUrlPrefix = 'data:image/png;base64,';

const XTypeGroup _profilePhotoTypes = XTypeGroup(
  label: 'Imágenes',
  extensions: <String>['jpg', 'jpeg', 'png', 'webp'],
  mimeTypes: <String>['image/jpeg', 'image/png', 'image/webp'],
);

/// Abre el selector nativo del sistema. Devuelve null si el usuario cancela.
Future<Uint8List?> pickProfilePhotoBytes() async {
  final file = await openFile(acceptedTypeGroups: const [_profilePhotoTypes]);
  if (file == null) return null;
  return file.readAsBytes();
}

String profilePhotoToDataUrl(Uint8List bytes) =>
    '$kProfilePhotoDataUrlPrefix${base64Encode(bytes)}';

/// Centro (0..1) limitado para que el recorte cuadrado nunca salga de la foto.
Offset profilePhotoCenterClamped({
  required int imageWidth,
  required int imageHeight,
  required double cropSide,
  required Offset center,
}) {
  final w = imageWidth.toDouble();
  final h = imageHeight.toDouble();
  if (w <= 0 || h <= 0) return const Offset(0.5, 0.5);
  final minX = (cropSide / 2) / w;
  final minY = (cropSide / 2) / h;
  return Offset(
    center.dx.clamp(minX, 1 - minX),
    center.dy.clamp(minY, 1 - minY),
  );
}

/// Rectángulo en píxeles de la imagen original que se usará como avatar.
Rect profilePhotoSourceRect({
  required int imageWidth,
  required int imageHeight,
  required double zoom,
  required Offset center,
}) {
  final w = imageWidth.toDouble();
  final h = imageHeight.toDouble();
  final safeZoom = zoom.clamp(1.0, kProfilePhotoMaxZoom);
  final side = math.min(w, h) / safeZoom;
  final clamped = profilePhotoCenterClamped(
    imageWidth: imageWidth,
    imageHeight: imageHeight,
    cropSide: side,
    center: center,
  );
  return Rect.fromLTWH(
    clamped.dx * w - side / 2,
    clamped.dy * h - side / 2,
    side,
    side,
  );
}

/// Diálogo de ajuste: arrastrar mueve la foto, los botones acercan/alejan.
Future<Uint8List?> showProfilePhotoCropDialog(
  BuildContext context,
  Uint8List bytes,
) async {
  final codec = await ui.instantiateImageCodec(bytes);
  final frame = await codec.getNextFrame();
  final image = frame.image;
  if (!context.mounted) {
    image.dispose();
    codec.dispose();
    return null;
  }
  try {
    return await showDialog<Uint8List>(
      context: context,
      barrierDismissible: false,
      barrierColor: AppColors.scrim,
      builder: (_) => _ProfilePhotoCropDialog(image: image),
    );
  } finally {
    image.dispose();
    codec.dispose();
  }
}

class _ProfilePhotoCropDialog extends StatefulWidget {
  const _ProfilePhotoCropDialog({required this.image});

  final ui.Image image;

  @override
  State<_ProfilePhotoCropDialog> createState() =>
      _ProfilePhotoCropDialogState();
}

class _ProfilePhotoCropDialogState extends State<_ProfilePhotoCropDialog> {
  double _zoom = 1;
  Offset _center = const Offset(0.5, 0.5);
  bool _saving = false;

  int get _width => widget.image.width;
  int get _height => widget.image.height;
  double get _cropSide => math.min(_width, _height) / _zoom;

  Rect get _sourceRect => profilePhotoSourceRect(
    imageWidth: _width,
    imageHeight: _height,
    zoom: _zoom,
    center: _center,
  );

  void _setZoom(double value) {
    final zoom = value.clamp(1.0, kProfilePhotoMaxZoom);
    setState(() {
      _zoom = zoom;
      _center = profilePhotoCenterClamped(
        imageWidth: _width,
        imageHeight: _height,
        cropSide: math.min(_width, _height) / zoom,
        center: _center,
      );
    });
  }

  void _drag(DragUpdateDetails details, double viewport) {
    final side = _cropSide;
    final scale = viewport / side;
    if (!scale.isFinite || scale <= 0) return;
    final next = Offset(
      _center.dx - details.delta.dx / scale / _width,
      _center.dy - details.delta.dy / scale / _height,
    );
    setState(() {
      _center = profilePhotoCenterClamped(
        imageWidth: _width,
        imageHeight: _height,
        cropSide: side,
        center: next,
      );
    });
  }

  Future<void> _confirm() async {
    if (_saving) return;
    setState(() => _saving = true);
    try {
      final bytes = await _cropToPng(widget.image, _sourceRect);
      if (mounted) Navigator.of(context).pop(bytes);
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final media = MediaQuery.sizeOf(context);
    final previewSide = math
        .min(420.0, math.min(media.width - 72, media.height - 320))
        .clamp(180.0, 420.0);

    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: const EdgeInsets.all(AppDimens.spaceLg),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 520),
        child: Container(
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidthThick,
            ),
            boxShadow: AppShadows.card,
          ),
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(AppDimens.spaceLg),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Row(
                  children: [
                    const Expanded(
                      child: Text(
                        'AJUSTAR FOTO',
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w900,
                          letterSpacing: 0.5,
                          color: AppColors.text,
                        ),
                      ),
                    ),
                    NeobrutalistIconButton(
                      icon: Icons.close_rounded,
                      tooltip: 'Cancelar',
                      size: 38,
                      onPressed: _saving
                          ? null
                          : () => Navigator.of(context).pop(),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceXs),
                const Text(
                  'Arrastra la imagen para elegir la porción que se verá en tu '
                  'avatar. Usa los botones para acercar o alejar.',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    height: 1.4,
                    color: AppColors.mutedStrong,
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                Center(
                  child: Container(
                    width: previewSide,
                    height: previewSide,
                    decoration: BoxDecoration(
                      color: AppColors.surfaceLow,
                      border: Border.all(
                        color: AppColors.border,
                        width: AppDimens.borderWidthThick,
                      ),
                      boxShadow: AppShadows.button,
                    ),
                    child: GestureDetector(
                      onPanUpdate: _saving
                          ? null
                          : (details) => _drag(details, previewSide),
                      child: MouseRegion(
                        cursor: SystemMouseCursors.grab,
                        child: CustomPaint(
                          painter: _ProfilePhotoCropPainter(
                            image: widget.image,
                            sourceRect: _sourceRect,
                          ),
                          child: const SizedBox.expand(),
                        ),
                      ),
                    ),
                  ),
                ),
                const SizedBox(height: AppDimens.spaceLg),
                Row(
                  children: [
                    NeobrutalistIconButton(
                      icon: Icons.remove_rounded,
                      tooltip: 'Alejar',
                      size: 38,
                      onPressed: _saving || _zoom <= 1
                          ? null
                          : () => _setZoom(_zoom - 0.25),
                    ),
                    const SizedBox(width: AppDimens.spaceSm),
                    Expanded(
                      child: Container(
                        padding: const EdgeInsets.symmetric(vertical: 10),
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          color: AppColors.bg,
                          border: Border.all(
                            color: AppColors.border,
                            width: AppDimens.borderWidth,
                          ),
                        ),
                        child: Text(
                          '${_zoom.toStringAsFixed(2)}x',
                          style: const TextStyle(
                            fontSize: 13,
                            fontWeight: FontWeight.w900,
                            color: AppColors.text,
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: AppDimens.spaceSm),
                    NeobrutalistIconButton(
                      icon: Icons.add_rounded,
                      tooltip: 'Acercar',
                      size: 38,
                      onPressed: _saving || _zoom >= kProfilePhotoMaxZoom
                          ? null
                          : () => _setZoom(_zoom + 0.25),
                    ),
                  ],
                ),
                const SizedBox(height: AppDimens.spaceLg),
                Row(
                  children: [
                    Expanded(
                      child: NeobrutalistButton(
                        label: 'CANCELAR',
                        variant: NeobrutalistButtonVariant.secondary,
                        expand: true,
                        onPressed: _saving
                            ? null
                            : () => Navigator.of(context).pop(),
                      ),
                    ),
                    const SizedBox(width: AppDimens.spaceMd),
                    Expanded(
                      child: NeobrutalistButton(
                        label: _saving ? 'PREPARANDO...' : 'USAR FOTO',
                        icon: Icons.check_rounded,
                        variant: NeobrutalistButtonVariant.accent,
                        expand: true,
                        onPressed: _saving ? null : _confirm,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

Future<Uint8List> _cropToPng(ui.Image image, Rect sourceRect) async {
  final recorder = ui.PictureRecorder();
  final canvas = Canvas(recorder);
  final side = kProfilePhotoOutputSize.toDouble();
  canvas.drawImageRect(
    image,
    sourceRect,
    Rect.fromLTWH(0, 0, side, side),
    Paint()..filterQuality = FilterQuality.high,
  );
  final picture = recorder.endRecording();
  final cropped = await picture.toImage(
    kProfilePhotoOutputSize,
    kProfilePhotoOutputSize,
  );
  final data = await cropped.toByteData(format: ui.ImageByteFormat.png);
  picture.dispose();
  cropped.dispose();
  if (data == null) {
    throw const FormatException('No se pudo preparar la imagen.');
  }
  return data.buffer.asUint8List();
}

class _ProfilePhotoCropPainter extends CustomPainter {
  const _ProfilePhotoCropPainter({
    required this.image,
    required this.sourceRect,
  });

  final ui.Image image;
  final Rect sourceRect;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawImageRect(
      image,
      sourceRect,
      Offset.zero & size,
      Paint()..filterQuality = FilterQuality.high,
    );

    final gridPaint = Paint()
      ..color = AppColors.surface.withAlpha(150)
      ..strokeWidth = 1;
    for (var i = 1; i < 3; i++) {
      final dx = size.width * i / 3;
      final dy = size.height * i / 3;
      canvas.drawLine(Offset(dx, 0), Offset(dx, size.height), gridPaint);
      canvas.drawLine(Offset(0, dy), Offset(size.width, dy), gridPaint);
    }
    canvas.drawRect(
      Offset.zero & size,
      Paint()
        ..color = AppColors.border
        ..style = PaintingStyle.stroke
        ..strokeWidth = AppDimens.borderWidthThick,
    );
  }

  @override
  bool shouldRepaint(covariant _ProfilePhotoCropPainter oldDelegate) =>
      oldDelegate.image != image || oldDelegate.sourceRect != sourceRect;
}
