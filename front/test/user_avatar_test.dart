import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/widgets/user_avatar.dart';

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
}
