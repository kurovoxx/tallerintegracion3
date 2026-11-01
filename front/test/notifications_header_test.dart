import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:taller_integracion_front/core/theme/app_theme.dart';
import 'package:taller_integracion_front/features/groups/notifications_bell.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  testWidgets(
    'encabezado NOTIFICACIONES: texto normal y con ancestro Material (360x800)',
    (tester) async {
      tester.view.physicalSize = const Size(360, 800);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(appBar: AppBar(actions: const [NotificationsBell()])),
        ),
      );
      await tester.pump(const Duration(milliseconds: 100));

      await tester.tap(find.byTooltip('Notificaciones'));
      await tester.pump();

      final header = tester.widget<Text>(find.text('NOTIFICACIONES'));
      expect(header.style?.color, AppColors.text);
      expect(header.style?.decoration, isNot(TextDecoration.underline));
      expect(
        find.ancestor(
          of: find.text('NOTIFICACIONES'),
          matching: find.byType(Material),
        ),
        findsWidgets,
      );
      expect(tester.takeException(), isNull);
    },
  );
}
