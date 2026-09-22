import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:taller_integracion_front/core/widgets/neobrutalism.dart';
import 'package:taller_integracion_front/features/groups/groups_screen.dart';

void main() {
  Future<void> pumpGroups(WidgetTester tester, Size size) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(const MaterialApp(home: GroupsScreen()));
    await tester.pumpAndSettle();
  }

  testWidgets('renderiza las credenciales de grupo en desktop', (tester) async {
    await pumpGroups(tester, const Size(1440, 900));

    expect(find.text('MIS GRUPOS'), findsOneWidget);
    expect(find.text('Cálculo II - Grupo Alpha'), findsOneWidget);
    expect(find.text('MAT1002'), findsOneWidget);
    expect(find.text('INF220'), findsOneWidget);
    expect(find.text('INF-360'), findsOneWidget);
    expect(find.text('5 integrantes'), findsOneWidget);
    expect(find.text('ABRIR'), findsNWidgets(3));
    expect(find.text('INVITAR'), findsNWidgets(3));
    expect(tester.takeException(), isNull);
  });

  testWidgets('crea un grupo desde el modal neobrutalista en movil', (
    tester,
  ) async {
    await pumpGroups(tester, const Size(390, 844));

    expect(find.byType(NeobrutalistFab), findsOneWidget);
    await tester.tap(find.byType(NeobrutalistFab));
    await tester.pumpAndSettle();
    expect(find.text('CREAR / UNIRSE A GRUPO'), findsOneWidget);

    await tester.enterText(find.byType(TextField).first, 'Equipo Física');
    await tester.tap(find.text('CREAR GRUPO'));
    await tester.pumpAndSettle();

    expect(find.text('Equipo Física'), findsOneWidget);
    expect(find.text('Grupo creado'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('telefono angosto renderiza sin desbordes', (tester) async {
    await pumpGroups(tester, const Size(320, 800));

    expect(find.text('MIS GRUPOS'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
