import 'package:flutter/material.dart';

class CoursesService extends ChangeNotifier {
  CoursesService._();

  static final CoursesService instance = CoursesService._();

  final List<Map<String, dynamic>> _courses = _initialCourses();

  static List<Map<String, dynamic>> _initialCourses() {
    return [
      {
        'id': 'calc-3',
        'code': 'INF-1111',
        'name': 'Cálculo III',
        'credits': 5,
        'status': kInProgress,
        'professor': 'Prof. 1',
        'schedule': 'Lun / Mié · 10:00–11:30',
        'room': 'CJP11-204',
        'requisite': 'REQ: MA1101',
        'requisite_alert': false,
        'progress': 68,
        'semester': 'Semestre 6',
        'icon': Icons.menu_book_rounded,
        'units': [
          {
            'title': 'Unidad 1: Funciones de Varias Variables y Derivadas Parciales',
            'badge': 'completado',
            'items': [
              {'title': 'Dominio, Rango y Graficación de Funciones de Varias Variables', 'checked': true, 'tag': 'Clase 1 – 4'},
              {'title': 'Límites y Continuidad en Rⁿ', 'checked': true, 'tag': 'Clase 5 – 7'},
            ]
          },
          {
            'title': 'Unidad 2: Integrales Múltiples y Cambio de Variables',
            'badge': 'en curso',
            'items': [
              {'title': 'Integrales Dobles sobre Regiones Generales', 'checked': true, 'tag': 'Clase 11 – 13'},
              {'title': 'Transformación a Coordenadas Polares, Cilíndricas y Esféricas', 'checked': false, 'tag': 'Clase 14 – 17'},
            ]
          },
          {
            'title': 'Unidad 3: Cálculo Vectorial y Teoremas Fundamentales',
            'badge': 'próximo',
            'items': [
              {'title': 'Campos Vectoriales, Divergencia y Rotacional', 'checked': false, 'tag': 'Clase 21 – 24'},
            ]
          },
        ],
        'materials': [
          {'name': 'Guía 1 - Derivadas Parciales.pdf', 'meta': 'PDF · 2.4 MB', 'icon': Icons.picture_as_pdf_rounded},
          {'name': 'Diapositivas - Integrales Dobles.pdf', 'meta': 'PDF · 5.1 MB', 'icon': Icons.picture_as_pdf_rounded},
          {'name': 'Solucionario Certamen 1 2025.pdf', 'meta': 'PDF · 1.8 MB', 'icon': Icons.picture_as_pdf_rounded},
        ],
        'grades': [
          {'name': 'Certamen 1 (25%) · Unidad 1', 'weight': 0.25, 'score': 62, 'status': 'graded'},
          {'name': 'Certamen 2 (35%) · Unidad 2', 'weight': 0.35, 'score': null, 'status': 'pending', 'date': '15 nov'},
          {'name': 'Tareas Prácticas y Talleres (40%)', 'weight': 0.40, 'score': 68, 'status': 'graded'},
        ],
      },
      {
        'id': 'taller-3',
        'code': 'INF-360',
        'name': 'Taller de Integración III',
        'credits': 4,
        'status': kPending,
        'professor': 'Prof. 2',
        'schedule': 'Mar / Jue · 08:30–10:00',
        'room': 'CJP11-102',
        'requisite': 'REQ: INF-200',
        'requisite_alert': false,
        'progress': 42,
        'semester': 'Semestre 6',
        'icon': Icons.computer_rounded,
        'units': [
          {
            'title': 'Unidad 1: Levantamiento de Requerimientos',
            'badge': 'en curso',
            'items': [
              {'title': 'Entrevistas y especificación funcional', 'checked': true, 'tag': 'Clase 1 – 3'},
              {'title': 'Historias de usuario y criterios de aceptación', 'checked': false, 'tag': 'Clase 4 – 6'},
            ]
          },
          {
            'title': 'Unidad 2: Diseño y Prototipado',
            'badge': 'próximo',
            'items': [
              {'title': 'Wireframes y PMN en Tailwind', 'checked': false, 'tag': 'Clase 7 – 10'},
            ]
          },
        ],
        'materials': [
          {'name': 'PMN - Especificación Sigma Academy.pdf', 'meta': 'PDF · 3.2 MB', 'icon': Icons.description_rounded},
          {'name': 'Guía Flutter Neo-Brutalismo.pdf', 'meta': 'PDF · 1.1 MB', 'icon': Icons.description_rounded},
        ],
        'grades': [
          {'name': 'Avance 1 - PMN (20%)', 'weight': 0.20, 'score': 58, 'status': 'graded'},
          {'name': 'Sprint 1 (30%)', 'weight': 0.30, 'score': null, 'status': 'pending'},
          {'name': 'Entrega Final (50%)', 'weight': 0.50, 'score': null, 'status': 'pending'},
        ],
      },
      {
        'id': 'seg-inf',
        'code': 'INF-350',
        'name': 'Seguridad Informática',
        'credits': 4,
        'status': kFailed,
        'professor': 'Prof. de la Vega',
        'schedule': 'Vie · 14:00–17:00',
        'room': 'CJP11-102',
        'requisite': 'REQ: INF-330',
        'requisite_alert': true,
        'progress': 25,
        'semester': 'Semestre 6',
        'icon': Icons.lock_rounded,
        'units': [
          {
            'title': 'Unidad 1: Criptografía',
            'badge': 'próximo',
            'items': [
              {'title': 'Cifrado simétrico y asimétrico', 'checked': false, 'tag': 'Clase 1 – 5'},
            ]
          },
        ],
        'materials': [
          {'name': 'Apunte Criptografía.pdf', 'meta': 'PDF · 4.0 MB', 'icon': Icons.picture_as_pdf_rounded},
        ],
        'grades': [
          {'name': 'Certamen 1 (40%)', 'weight': 0.40, 'score': 35, 'status': 'graded'},
          {'name': 'Laboratorio (60%)', 'weight': 0.60, 'score': null, 'status': 'pending'},
        ],
      },
      {
        'id': 'redes',
        'code': 'INF-330',
        'name': 'Redes de Computadores',
        'credits': 5,
        'status': kApproved,
        'professor': 'Profesora Morales',
        'schedule': 'Lun / Jue · 11:30–13:00',
        'room': 'CJP11-101',
        'requisite': 'REQ: INF-100',
        'requisite_alert': false,
        'progress': 75,
        'semester': 'Semestre 6',
        'icon': Icons.public_rounded,
        'units': [
          {
            'title': 'Unidad 1: Modelo OSI y TCP/IP',
            'badge': 'completado',
            'items': [
              {'title': 'Capas y encapsulamiento', 'checked': true, 'tag': 'Clase 1 – 4'},
            ]
          },
          {
            'title': 'Unidad 2: Enrutamiento',
            'badge': 'completado',
            'items': [
              {'title': 'Algoritmos de enrutamiento', 'checked': true, 'tag': 'Clase 5 – 8'},
            ]
          },
        ],
        'materials': [
          {'name': 'Guía Laboratorio Redes.pdf', 'meta': 'PDF · 2.0 MB', 'icon': Icons.description_rounded},
        ],
        'grades': [
          {'name': 'Certamen 1 (30%)', 'weight': 0.30, 'score': 55, 'status': 'graded'},
          {'name': 'Certamen 2 (30%)', 'weight': 0.30, 'score': 60, 'status': 'graded'},
          {'name': 'Proyecto (40%)', 'weight': 0.40, 'score': 62, 'status': 'graded'},
        ],
      },
    ];
  }

  List<Map<String, dynamic>> get courses => List.unmodifiable(_courses);

  void addCourse(Map<String, dynamic> course) {
    _courses.insert(0, course);
    notifyListeners();
  }

  void updateCourse(String id, Map<String, dynamic> updates) {
    final index = _courses.indexWhere((c) => c['id'] == id);
    if (index == -1) return;
    _courses[index] = Map<String, dynamic>.from(_courses[index])
      ..addAll(updates);
    notifyListeners();
  }

  void removeCourse(String id) {
    final before = _courses.length;
    _courses.removeWhere((c) => c['id'] == id);
    if (_courses.length != before) notifyListeners();
  }

  void reset() {
    _courses
      ..clear()
      ..addAll(_initialCourses());
    notifyListeners();
  }
}

const String kInProgress = 'in_progress';
const String kPending = 'pending';
const String kApproved = 'approved';
const String kFailed = 'failed';