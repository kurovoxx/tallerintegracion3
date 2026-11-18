import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';

void main() {
  const id = 'a8kp92mqwx7r';

  group('parsePublicIdInput acepta solo el código público', () {
    final accepted = <String, String>{
      'el código crudo': id,
      'el código con espacios': '  $id  ',
      'el código en mayúsculas': id.toUpperCase(),
    };
    accepted.forEach((name, input) {
      test(name, () {
        final r = parsePublicIdInput(input);
        expect(r.isValid, isTrue, reason: input);
        expect(r.publicId, id);
      });
    });
  });

  group('parsePublicIdInput rechaza', () {
    final rejected = <String, PublicIdInputError>{
      '': PublicIdInputError.empty,
      '   ': PublicIdInputError.empty,
      '550e8400-e29b-41d4-a716-446655440000': PublicIdInputError.internalUuid,
      // El flujo nuevo no usa enlaces: ni los antiguos sigma-academy://perfil/
      // ni enlaces web se aceptan, aunque contengan un código válido.
      'sigma-academy://perfil/$id': PublicIdInputError.link,
      'Mira mi perfil: sigma-academy://perfil/$id': PublicIdInputError.link,
      'https://sigma.example/perfil/$id': PublicIdInputError.link,
      'http://localhost/perfil/$id': PublicIdInputError.link,
      'otro://perfil/$id': PublicIdInputError.link,
      'a8kp92mqwx7l': PublicIdInputError.invalid,
      'a8kp92mqwx7': PublicIdInputError.invalid,
      'Ana Pérez': PublicIdInputError.invalid,
      'ana@uct.cl': PublicIdInputError.invalid,
      '+56912345678': PublicIdInputError.invalid,
      'perfil $id': PublicIdInputError.invalid,
    };
    rejected.forEach((input, error) {
      test('«$input»', () {
        final r = parsePublicIdInput(input);
        expect(r.isValid, isFalse);
        expect(r.error, error);
        expect(publicIdInputMessage(r.error!), isNotEmpty);
      });
    });
  });

  test('los mensajes no usan términos técnicos ni el esquema antiguo', () {
    for (final error in PublicIdInputError.values) {
      final message = publicIdInputMessage(error);
      for (final technical in <String>[
        'UUID',
        'user_id',
        'identificador interno',
        'sigma-academy://',
      ]) {
        expect(message, isNot(contains(technical)), reason: '$error');
      }
    }
    expect(
      publicIdInputMessage(PublicIdInputError.empty),
      'Ingresa el código del perfil que deseas buscar.',
    );
  });

  group('PublicProfile.fromJson', () {
    test('perfil público expone todos los campos solicitados', () {
      final p = PublicProfile.fromJson(<String, dynamic>{
        'public_id': id,
        'display_name': 'Ana',
        'visibility': 'public',
        'description': 'Me gusta el cálculo',
        'email': 'ana@test.invalid',
        'phone': '+56911111111',
        'institution': 'UCT',
        'photo_url': 'data:image/png;base64,iVBORw0KGgo=',
      });
      expect(p.isPublic, isTrue);
      expect(p.description, 'Me gusta el cálculo');
      expect(p.email, 'ana@test.invalid');
      expect(p.phone, '+56911111111');
      expect(p.institution, 'UCT');
      expect(p.photoDataUrl, isNotNull);
    });

    test('perfil privado conserva solo nombre y foto aunque lleguen más', () {
      final p = PublicProfile.fromJson(<String, dynamic>{
        'public_id': id,
        'display_name': 'Bea',
        'visibility': 'private',
        'description': 'no',
        'email': 'bea@test.invalid',
        'phone': '+56900000000',
        'institution': 'UCT',
        'photo_url': 'data:image/png;base64,iVBORw0KGgo=',
      });
      expect(p.isPublic, isFalse);
      expect(p.displayName, 'Bea');
      expect(p.photoDataUrl, isNotNull);
      expect(p.description, isNull);
      expect(p.email, isNull);
      expect(p.phone, isNull);
      expect(p.institution, isNull);
    });
  });
}
