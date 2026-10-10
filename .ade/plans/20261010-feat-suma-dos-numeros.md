# Plan: Script sencillo para sumar 2 números (prueba de flujo Planning -> Coder)

## 1. Contexto & Objetivo
Probar el flujo de automatización multi-agente Planning -> Coder con una tarea mínima y sin riesgo para el proyecto.
Se busca un script sencillo, aislado del resto del código (sin tocar `back/`, `front/`, ni schemas), que sume dos números y demuestre el handoff por archivo en `.ade/plans/`.

## 2. Criterios de Aceptación
- [ ] Existe el archivo `scripts/suma_dos_numeros.py`.
- [ ] Expone una función `sumar(a, b)` que retorna la suma numérica de `a` y `b`.
- [ ] Soporta ejecución por CLI: `python3 scripts/suma_dos_numeros.py 2 3` imprime `5` (solo el resultado, sin texto extra).
- [ ] Acepta enteros y decimales (ej: `2.5 + 3.2 = 5.7`).
- [ ] No modifica ningún otro archivo del repo.
- [ ] El script ejecuta sin dependencias externas (solo stdlib Python3).

## 3. Paso a Paso para Programación
1. Leer este plan completo antes de tocar código.
2. Crear el archivo `scripts/suma_dos_numeros.py`.
3. Implementar `def sumar(a, b):` que convierta a `float` si es necesario y retorne `a + b`.
4. Implementar bloque `if __name__ == "__main__":` que:
   - Lea `sys.argv[1]` y `sys.argv[2]` como números.
   - Llame a `sumar()` e imprima el resultado (si es entero, sin `.0` innecesario).
   - Si faltan argumentos o no son numéricos, imprima uso en `stderr` y salga con código `1`.
5. Verificar manualmente: `python3 scripts/suma_dos_numeros.py 2 3` debe imprimir `5`.
6. Reportar archivos creados/modificados y lista de pruebas para el Agente de Testeo. No agregar tests automatizados en este plan de prueba.

## 4. Casos de Borde & Pruebas Sugeridas
- `2 + 3 = 5` (caso feliz enteros).
- `2.5 + 3.2 = 5.7` (decimales).
- `-1 + 5 = 4` (negativos).
- `0 + 0 = 0`.
- Sin argumentos -> error uso + exit 1.
- Argumentos no numéricos (`a b`) -> error + exit 1.
- Números grandes (`999999999 + 1 = 1000000000`).
- Directriz para Testing: validar salida exacta por stdout y código de salida, sin dependencias externas.
