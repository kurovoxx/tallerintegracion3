# GET /health

Chequeo de salud básico del microservicio Auth/Identity.

## 1. Req (Petición)
- **Método y Ruta:** `GET /health` (Pública; `back/auth/cmd/server/main.go:94`).
- **Headers:** Ninguno requerido.

## 2. Res (Respuestas y Códigos HTTP)
- **`200 OK`:** `{"status":"ok","service":"auth"}`.

## 3. Invocación (Front / Otros Servicios)
- **Kubernetes:** Probes de Liveness y Readiness en `k8s/manifests.yaml`.

## 4. Flujo Técnico (Paso a Paso)
- Retorna 200 en memoria sin acceder a la base de datos.
