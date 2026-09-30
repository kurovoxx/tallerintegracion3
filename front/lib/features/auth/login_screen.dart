import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/common_widgets.dart';
import '../../core/services/auth_service.dart';
import '../../core/services/session_manager.dart';
import '../../core/theme/app_theme.dart';
import '../../core/widgets/main_shell.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  static const String _rememberedEmailKey = 'auth_remembered_email';

  bool isLoginTab = true;
  final _authService = AuthService();
  bool _isSubmittingLogin = false;

  final _loginFormKey = GlobalKey<FormState>();
  final TextEditingController _loginEmailController = TextEditingController();
  final TextEditingController _loginPasswordController =
      TextEditingController();
  bool _obscureLoginPassword = true;
  bool _rememberMe = true;

  final _regFormKey = GlobalKey<FormState>();
  final TextEditingController _regNameController = TextEditingController();
  final TextEditingController _regEmailController = TextEditingController();
  // Mapea directo a identity.profiles.institution (no existe columna "career").
  final TextEditingController _regInstitutionController =
      TextEditingController();
  final TextEditingController _regPasswordController = TextEditingController();
  bool _obscureRegPassword = true;
  bool _isSubmittingRegister = false;

  // RN-1.3 / RF-01: el registro crea exclusivamente cuentas de estudiante.
  // El rol queda fijado de manera definitiva e inmutable; no se expone ningún
  // selector ni vuelve a ser editable desde el perfil.
  static const String _selectedRole = 'student';

  @override
  void initState() {
    super.initState();
    _restoreRememberedEmail();
  }

  Future<void> _restoreRememberedEmail() async {
    final preferences = await SharedPreferences.getInstance();
    final savedEmail = preferences.getString(_rememberedEmailKey);
    if (!mounted || savedEmail == null || savedEmail.isEmpty) return;
    setState(() {
      _loginEmailController.text = savedEmail;
      _rememberMe = true;
    });
  }

  Future<void> _persistRememberedEmail(String email) async {
    final preferences = await SharedPreferences.getInstance();
    if (_rememberMe) {
      await preferences.setString(_rememberedEmailKey, email);
    } else {
      await preferences.remove(_rememberedEmailKey);
    }
  }

  @override
  void dispose() {
    _loginEmailController.dispose();
    _loginPasswordController.dispose();
    _regNameController.dispose();
    _regEmailController.dispose();
    _regInstitutionController.dispose();
    _regPasswordController.dispose();
    _authService.dispose();
    super.dispose();
  }

  Future<void> _submitLogin() async {
    if (!_loginFormKey.currentState!.validate()) return;
    if (_isSubmittingLogin) return;

    setState(() => _isSubmittingLogin = true);

    try {
      final result = await _authService.login(
        email: _loginEmailController.text.trim(),
        password: _loginPasswordController.text,
      );

      if (!mounted) return;

      if (result.success) {
        // Guardar sesión para carga híbrida de notas (backend + local).
        // Se guarda también el refresh_token para renovar sin re-login.
        final token =
            (result.data?['access_token'] as String?) ??
            (result.data?['token'] as String?) ??
            '';
        if (token.isNotEmpty) {
          await SessionManager.saveSession(
            token,
            result.data,
            rememberMe: _rememberMe,
            refreshToken: result.data?['refresh_token'] as String?,
          );
        }
        await _persistRememberedEmail(_loginEmailController.text.trim());
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(
              'INGRESO CORRECTO',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: Colors.white,
              ),
            ),
            backgroundColor: AppColors.border,
          ),
        );

        Navigator.of(context).pushReplacement(
          MaterialPageRoute(
            builder: (_) => MainShell(
              userData: result.data,
              initialIndex: 7, // Perfil de Usuario
            ),
          ),
        );
        return;
      }

      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            result.message ?? 'Ocurrió un error al iniciar sesión.',
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              color: Colors.white,
            ),
          ),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _isSubmittingLogin = false);
    }
  }

  Future<void> _submitRegister() async {
    if (!_regFormKey.currentState!.validate()) return;
    if (_isSubmittingRegister) return;

    setState(() => _isSubmittingRegister = true);

    try {
      final result = await _authService.register(
        email: _regEmailController.text.trim(),
        password: _regPasswordController.text,
        displayName: _regNameController.text.trim(),
        institution: _regInstitutionController.text.trim(),
        role: _selectedRole, // RF-01: cuenta de estudiante, inmutable
      );

      if (!mounted) return;

      if (result.success) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text(
              'CUENTA CREADA - YA PUEDES INICIAR SESIÓN',
              style: TextStyle(
                fontWeight: FontWeight.w800,
                color: Colors.white,
              ),
            ),
            backgroundColor: AppColors.border,
          ),
        );
        setState(() {
          isLoginTab = true;
          _regPasswordController.clear();
        });
        return;
      }

      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            result.message ?? 'Error al crear cuenta',
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              color: Colors.white,
            ),
          ),
          backgroundColor: AppColors.error,
        ),
      );
    } finally {
      if (mounted) setState(() => _isSubmittingRegister = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final screenWidth = MediaQuery.of(context).size.width;
    final isDesktop = screenWidth > AppDimens.breakpointDesktop;

    return Scaffold(
      backgroundColor: AppColors.surface,
      body: SafeArea(
        child: SizedBox.expand(
          child: isDesktop
              ? Row(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    SizedBox(
                      width: 500,
                      child: Container(
                        decoration: const BoxDecoration(
                          color: AppColors.surface,
                          border: Border(
                            right: BorderSide(
                              color: AppColors.border,
                              width: AppDimens.borderWidth,
                            ),
                          ),
                          boxShadow: [
                            BoxShadow(
                              color: AppColors.border,
                              offset: Offset(4, 0),
                              blurRadius: 0,
                            ),
                          ],
                        ),
                        child: _buildLeftPanelContent(isDesktop: true),
                      ),
                    ),
                    Expanded(
                      child: Container(
                        color: AppColors.accentYellow,
                        child: _buildRightPanelContent(),
                      ),
                    ),
                  ],
                )
              : Container(
                  color: AppColors.surface,
                  child: _buildLeftPanelContent(isDesktop: false),
                ),
        ),
      ),
    );
  }

  Widget _buildLeftPanelContent({required bool isDesktop}) {
    return SingleChildScrollView(
      padding: EdgeInsets.symmetric(
        horizontal: isDesktop ? 40 : 24,
        vertical: isDesktop ? 48 : 28,
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          _buildBrandHeader(),
          if (!isDesktop) ...[
            const SizedBox(height: 20),
            _buildMobileHeroBanner(),
          ],
          const SizedBox(height: 28),
          _buildAuthTabs(),
          const SizedBox(height: 28),
          isLoginTab ? _buildLoginForm() : _buildRegisterForm(),
          if (!isDesktop) ...[
            const SizedBox(height: 32),
            _buildMobileFeatureStrip(),
          ],
        ],
      ),
    );
  }

  Widget _buildBrandHeader() {
    return Row(
      children: [
        Container(
          width: 52,
          height: 52,
          decoration: BoxDecoration(
            color: AppColors.accentYellow,
            border: Border.all(
              color: AppColors.border,
              width: AppDimens.borderWidth,
            ),
            borderRadius: BorderRadius.circular(AppDimens.radius),
            boxShadow: const [
              BoxShadow(
                color: AppColors.border,
                offset: Offset(3, 3),
                blurRadius: 0,
              ),
            ],
          ),
          child: const Center(
            child: Text(
              'S',
              style: TextStyle(
                fontSize: 22,
                fontWeight: FontWeight.w900,
                color: AppColors.text,
              ),
            ),
          ),
        ),
        const SizedBox(width: 16),
        const Expanded(
          child: Text(
            'SIGMA ACADEMY',
            style: TextStyle(
              fontSize: 22,
              fontWeight: FontWeight.w900,
              letterSpacing: -0.5,
              color: AppColors.text,
              height: 1.1,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildMobileHeroBanner() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 16),
      decoration: BoxDecoration(
        color: AppColors.accentYellow,
        border: Border.all(
          color: AppColors.border,
          width: AppDimens.borderWidth,
        ),
        borderRadius: BorderRadius.circular(AppDimens.radius),
        boxShadow: const [
          BoxShadow(
            color: AppColors.border,
            offset: Offset(3, 3),
            blurRadius: 0,
          ),
        ],
      ),
      child: Row(
        children: [
          Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: AppColors.surface,
              border: Border.all(
                color: AppColors.border,
                width: AppDimens.borderWidth,
              ),
              borderRadius: BorderRadius.circular(AppDimens.radius),
              boxShadow: const [
                BoxShadow(
                  color: AppColors.border,
                  offset: Offset(2, 2),
                  blurRadius: 0,
                ),
              ],
            ),
            child: const Icon(
              Icons.rocket_launch_rounded,
              color: AppColors.text,
              size: 20,
            ),
          ),
          const SizedBox(width: 14),
          const Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'ORGANIZA TU SEMESTRE',
                  style: TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w900,
                    color: AppColors.text,
                  ),
                ),
                SizedBox(height: 3),
                Text(
                  'Malla, contenidos y grupos en un solo lugar.',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    color: AppColors.text,
                    height: 1.25,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildMobileFeatureStrip() {
    final items = [
      (Icons.grid_view_rounded, 'MALLA'),
      (Icons.groups_rounded, 'GRUPOS'),
      (Icons.description_rounded, 'CONTENIDOS'),
    ];
    return Row(
      children: [
        for (var i = 0; i < items.length; i++) ...[
          if (i > 0) const SizedBox(width: 10),
          Expanded(
            child: Container(
              padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 6),
              decoration: BoxDecoration(
                color: AppColors.bg,
                border: Border.all(
                  color: AppColors.border,
                  width: AppDimens.borderWidth,
                ),
                borderRadius: BorderRadius.circular(AppDimens.radius),
                boxShadow: const [
                  BoxShadow(
                    color: AppColors.border,
                    offset: Offset(2, 2),
                    blurRadius: 0,
                  ),
                ],
              ),
              child: Column(
                children: [
                  Icon(items[i].$1, color: AppColors.text, size: 20),
                  const SizedBox(height: 6),
                  Text(
                    items[i].$2,
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.w900,
                      color: AppColors.text,
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ],
    );
  }

  Widget _buildAuthTabs() {
    return Container(
      decoration: const BoxDecoration(
        border: Border(
          bottom: BorderSide(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: _tabButton(
              'INICIAR SESIÓN',
              isLoginTab,
              () => setState(() => isLoginTab = true),
            ),
          ),
          const SizedBox(width: 6),
          Expanded(
            child: _tabButton(
              'CREAR CUENTA',
              !isLoginTab,
              () => setState(() => isLoginTab = false),
            ),
          ),
        ],
      ),
    );
  }

  Widget _tabButton(String label, bool active, VoidCallback onTap) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(vertical: 12),
        decoration: BoxDecoration(
          color: active ? AppColors.accentYellow : AppColors.bg,
          border: Border.all(
            color: AppColors.border,
            width: AppDimens.borderWidth,
          ),
          borderRadius: const BorderRadius.only(
            topLeft: Radius.circular(AppDimens.radius),
            topRight: Radius.circular(AppDimens.radius),
          ),
          boxShadow: active
              ? const [
                  BoxShadow(
                    color: AppColors.border,
                    offset: Offset(2, -2),
                    blurRadius: 0,
                  ),
                ]
              : null,
        ),
        child: Text(
          label,
          textAlign: TextAlign.center,
          style: const TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w900,
            letterSpacing: 0.5,
            color: AppColors.text,
          ),
        ),
      ),
    );
  }

  Widget _buildLoginForm() {
    return Form(
      key: _loginFormKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const AppFieldLabel('CORREO INSTITUCIONAL'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _loginEmailController,
            keyboardType: TextInputType.emailAddress,
            textInputAction: TextInputAction.next,
            autofillHints: const [AutofillHints.email],
            inputFormatters: [
              FilteringTextInputFormatter.deny(RegExp(r'\s')),
              LengthLimitingTextInputFormatter(120),
            ],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('ejemplo@alu.uct.cl'),
            validator: (v) {
              final email = v?.trim() ?? '';
              if (email.isEmpty) return 'El correo es requerido';
              if (!RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$').hasMatch(email))
                return 'Ingresa un correo válido';
              return null;
            },
          ),
          const SizedBox(height: 18),
          const AppFieldLabel('CONTRASEÑA'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _loginPasswordController,
            obscureText: _obscureLoginPassword,
            textInputAction: TextInputAction.done,
            autofillHints: const [AutofillHints.password],
            // FIX: ya no se bloquean espacios. bcrypt soporta passphrases
            // con espacios hasta 72 bytes; solo se limita el largo máximo.
            inputFormatters: [LengthLimitingTextInputFormatter(72)],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('••••••••').copyWith(
              suffixIcon: IconButton(
                icon: Icon(
                  _obscureLoginPassword
                      ? Icons.visibility_outlined
                      : Icons.visibility_off_outlined,
                  color: AppColors.muted,
                  size: 20,
                ),
                onPressed: () => setState(
                  () => _obscureLoginPassword = !_obscureLoginPassword,
                ),
              ),
            ),
            validator: (v) {
              if (v == null || v.isEmpty) return 'La contraseña es requerida';
              if (v.length < 6) return 'Mínimo 6 caracteres';
              return null;
            },
            onFieldSubmitted: (_) {
              if (!_isSubmittingLogin) {
                _submitLogin();
              }
            },
          ),
          const SizedBox(height: 16),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Row(
                children: [
                  SizedBox(
                    width: 20,
                    height: 20,
                    child: Checkbox(
                      value: _rememberMe,
                      activeColor: AppColors.accentYellow,
                      checkColor: AppColors.text,
                      side: const BorderSide(color: AppColors.border, width: 2),
                      onChanged: (val) =>
                          setState(() => _rememberMe = val ?? false),
                    ),
                  ),
                  const SizedBox(width: 8),
                  const Text(
                    'RECORDARME',
                    style: TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w800,
                      color: AppColors.text,
                    ),
                  ),
                ],
              ),
              InkWell(
                onTap: () {},
                child: const Text(
                  '¿Olvidaste tu clave?',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                    color: AppColors.accentBlue,
                    decoration: TextDecoration.underline,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 28),
          SubmitButton(
            text: _isSubmittingLogin ? 'INGRESANDO...' : 'INGRESAR A LA PAGINA',
            onPressed: _isSubmittingLogin ? () {} : _submitLogin,
          ),
        ],
      ),
    );
  }

  Widget _buildRegisterForm() {
    return Form(
      key: _regFormKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const AppFieldLabel('NOMBRE COMPLETO'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _regNameController,
            keyboardType: TextInputType.name,
            textInputAction: TextInputAction.next,
            autofillHints: const [AutofillHints.name],
            inputFormatters: [LengthLimitingTextInputFormatter(100)],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('Miguel Fernandez'),
            validator: (v) {
              final name = v?.trim() ?? '';
              if (name.isEmpty) return 'El nombre es requerido';
              if (name.length < 3) return 'Ingresa al menos 3 caracteres';
              return null;
            },
          ),
          const SizedBox(height: 16),

          const AppFieldLabel('CORREO INSTITUCIONAL'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _regEmailController,
            keyboardType: TextInputType.emailAddress,
            textInputAction: TextInputAction.next,
            autofillHints: const [AutofillHints.email],
            inputFormatters: [
              FilteringTextInputFormatter.deny(RegExp(r'\s')),
              LengthLimitingTextInputFormatter(120),
            ],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('ejemplo@alu.uct.cl'),
            validator: (v) {
              final email = v?.trim() ?? '';
              if (email.isEmpty) return 'El correo es requerido';
              if (!RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$').hasMatch(email))
                return 'Correo inválido';
              return null;
            },
          ),
          const SizedBox(height: 16),

          // Mapea a identity.profiles.institution (no existe columna "career"
          // en el MER). Se deja el hint explícito para no confundir al usuario.
          const AppFieldLabel('INSTITUCIÓN / CASA DE ESTUDIOS'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _regInstitutionController,
            textInputAction: TextInputAction.next,
            inputFormatters: [LengthLimitingTextInputFormatter(200)],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration(
              'Ej. Universidad Católica de Temuco',
            ),
            validator: (v) {
              final institution = v?.trim() ?? '';
              if (institution.isEmpty) return 'La institución es requerida';
              if (institution.length < 3)
                return 'Ingresa una institución válida';
              return null;
            },
          ),
          const SizedBox(height: 16),

          const AppFieldLabel('CONTRASEÑA'),
          const SizedBox(height: 6),
          TextFormField(
            controller: _regPasswordController,
            obscureText: _obscureRegPassword,
            textInputAction: TextInputAction.done,
            autofillHints: const [AutofillHints.newPassword],
            // FIX: ya no se bloquean espacios (passphrases seguras válidas).
            inputFormatters: [LengthLimitingTextInputFormatter(72)],
            style: const TextStyle(
              fontWeight: FontWeight.w700,
              fontSize: 14,
              color: AppColors.text,
            ),
            decoration: appInputDecoration('••••••••').copyWith(
              suffixIcon: IconButton(
                tooltip: _obscureRegPassword
                    ? 'Mostrar contraseña'
                    : 'Ocultar contraseña',
                icon: Icon(
                  _obscureRegPassword
                      ? Icons.visibility_outlined
                      : Icons.visibility_off_outlined,
                  color: AppColors.muted,
                  size: 20,
                ),
                onPressed: () =>
                    setState(() => _obscureRegPassword = !_obscureRegPassword),
              ),
            ),
            validator: (v) {
              if (v == null || v.isEmpty) return 'La contraseña es requerida';
              if (v.length < 6) return 'Mínimo 6 caracteres';
              return null;
            },
            onFieldSubmitted: (_) {
              if (!_isSubmittingRegister) {
                _submitRegister();
              }
            },
          ),
          const SizedBox(height: 28),
          SubmitButton(
            text: _isSubmittingRegister ? 'CREANDO...' : 'CREAR MI CUENTA',
            onPressed: _isSubmittingRegister ? () {} : _submitRegister,
          ),
        ],
      ),
    );
  }

  Widget _buildRightPanelContent() {
    return SingleChildScrollView(
      padding: const EdgeInsets.symmetric(horizontal: 56, vertical: 48),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Text(
            'ORGANIZA',
            style: TextStyle(
              fontSize: 36,
              fontWeight: FontWeight.w900,
              letterSpacing: -0.8,
              color: AppColors.text,
            ),
          ),
          const SizedBox(height: 12),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 580),
            child: const Text(
              'Gestiona tus asignaturas, controla asistencias y calcula notas para asegurar tu semestre universitario.',
              style: TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
                color: AppColors.text,
                height: 1.4,
              ),
            ),
          ),
          const SizedBox(height: 36),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 820),
            child: LayoutBuilder(
              builder: (context, constraints) {
                final useTwoCols = constraints.maxWidth > 560;
                final cardWidth = useTwoCols
                    ? (constraints.maxWidth - 20) / 2
                    : constraints.maxWidth;
                return Wrap(
                  spacing: 20,
                  runSpacing: 20,
                  children: [
                    SizedBox(
                      width: cardWidth,
                      child: const FeatureCard(
                        icon: Icons.grid_view_rounded,
                        title: 'MI MALLA CURRICULAR',
                        desc:
                            'Planifica los ramos y desbloquea asignaturas semestrales.',
                      ),
                    ),
                    SizedBox(
                      width: cardWidth,
                      child: const FeatureCard(
                        icon: Icons.groups_rounded,
                        title: 'GRUPOS DE ESTUDIO',
                        desc: 'Conéctate y comparte material con compañeros.',
                      ),
                    ),
                    SizedBox(
                      width: cardWidth,
                      child: const FeatureCard(
                        icon: Icons.calculate_rounded,
                        title: 'SIMULADOR DE NOTAS',
                        desc:
                            'Calcula exactamente la calificación requerida para aprobar.',
                      ),
                    ),
                  ],
                );
              },
            ),
          ),
        ],
      ),
    );
  }
}
