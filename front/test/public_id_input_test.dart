import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/models/profile_models.dart';

void main() {
  const id = 'a8kp92mqwx7r';

  group('parsePublicIdInput acepta', () {
    final accepted = <String, String>{
      'el ID crudo': id,
      'el ID con espacios': '  $id  ',
      'el ID en mayúsculas': id.toUpperCase(),
      'el enlace interno': 'sigma-academy://perfil/$id',
      'el enlace con mayúsculas': 'SIGMA-ACADEMY://PERFIL/${id.toUpperCase()}',
      'el enlace dentro de otro texto':
          'Mira mi perfil: sigma-academy://perfil/$id gracias',
      'el enlace seguido de puntuación':
          'Mi perfil (sigma-academy://perfil/$id).',
      'el mismo enlace repetido':
          'sigma-academy://perfil/$id y otra vez sigma-academy://perfil/$id',
      'un enlace web junto a un enlace interno':
          'web https://example.com y perfil sigma-academy://perfil/$id',
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
      'https://sigma.example/perfil/$id': PublicIdInputError.webLink,
      'http://localhost/perfil/$id': PublicIdInputError.webLink,
      'https://evil.example/?next=sigma-academy://perfil/$id':
          PublicIdInputError.webLink,
      'otro://perfil/$id': PublicIdInputError.unsupportedLink,
      'sigma-academy://usuarios/$id': PublicIdInputError.unsupportedLink,
      'sigma-academy://perfil/$id/apuntes': PublicIdInputError.unsupportedLink,
      'sigma-academy://perfil/$id/': PublicIdInputError.unsupportedLink,
      'sigma-academy://perfil/$id?x=1': PublicIdInputError.unsupportedLink,
      'sigma-academy://perfil/550e8400-e29b-41d4-a716-446655440000':
          PublicIdInputError.unsupportedLink,
      'sigma-academy://perfil/$id sigma-academy://perfil/b8kp92mqwx7r':
          PublicIdInputError.ambiguous,
      'a8kp92mqwx7l': PublicIdInputError.invalid,
      'a8kp92mqwx7': PublicIdInputError.invalid,
      'Ana Pérez': PublicIdInputError.invalid,
      'ana@uct.cl': PublicIdInputError.invalid,
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
}
