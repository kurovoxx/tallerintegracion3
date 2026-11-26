import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/widgets/user_avatar.dart';

const _png =
    'data:image/png;base64,'
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk'
    '+A8AAQUBAScY42YAAAAASUVORK5CYII=';

void main() {
  test('iniciales de nombre y correo', () {
    expect(userInitials('Ana Admin'), 'AA');
    expect(userInitials('ana@example.com'), 'AN');
    expect(userInitials(''), '?');
  });

  test('resuelve data URL y URL remota como ImageProvider', () {
    final memory = userAvatarProvider('data:image/png;base64,iVBORw0KGgo=');
    expect(memory, isNotNull);
    final network = userAvatarProvider('https://cdn.test/thumb.png');
    expect(network, isNotNull);
    expect(userAvatarProvider(''), isNull);
    expect(userAvatarProvider('data:image/png,sin-base64'), isNull);
  });

  testWidgets('reutiliza el provider al reconstruir con la misma foto', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: UserAvatar(name: 'Ana', size: 36, photoUrl: _png),
      ),
    );
    final first = tester.widget<Image>(find.byType(Image)).image;

    await tester.pumpWidget(
      const MaterialApp(
        home: UserAvatar(name: 'Ana Admin', size: 36, photoUrl: _png),
      ),
    );
    final second = tester.widget<Image>(find.byType(Image)).image;

    expect(identical(first, second), isTrue);
  });
}
