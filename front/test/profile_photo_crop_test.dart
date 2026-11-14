import 'dart:convert';
import 'dart:typed_data';
import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/features/academic/profile_photo_crop.dart';

void main() {
  test('foto horizontal: recorte cuadrado centrado usa el alto completo', () {
    final rect = profilePhotoSourceRect(
      imageWidth: 1200,
      imageHeight: 800,
      zoom: 1,
      center: const Offset(0.5, 0.5),
    );
    expect(rect, const Rect.fromLTWH(200, 0, 800, 800));
  });

  test('zoom reduce el recorte alrededor del centro elegido', () {
    final rect = profilePhotoSourceRect(
      imageWidth: 1200,
      imageHeight: 800,
      zoom: 2,
      center: const Offset(0.5, 0.5),
    );
    expect(rect, const Rect.fromLTWH(400, 200, 400, 400));
  });

  test('el centro se limita para no salir de la imagen', () {
    final center = profilePhotoCenterClamped(
      imageWidth: 1200,
      imageHeight: 800,
      cropSide: 800,
      center: const Offset(0, 1),
    );
    expect(center.dx, closeTo(1 / 3, 0.0001));
    expect(center.dy, closeTo(0.5, 0.0001));
    final rect = profilePhotoSourceRect(
      imageWidth: 1200,
      imageHeight: 800,
      zoom: 1,
      center: center,
    );
    expect(rect.left, 0);
    expect(rect.top, 0);
    expect(rect.right, 800);
    expect(rect.bottom, 800);
  });

  test('convierte bytes PNG a data URL', () {
    final dataUrl = profilePhotoToDataUrl(Uint8List.fromList([1, 2, 3, 4]));
    expect(dataUrl, startsWith('data:image/png;base64,'));
    expect(base64Decode(dataUrl.split(',').last), [1, 2, 3, 4]);
  });
}
