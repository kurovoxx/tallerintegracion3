# Comunicación entre Microservicios: gRPC y Protocol Buffers

Este documento detalla el flujo técnico completo de gRPC y Protocol Buffers en el proyecto: la definición de contratos, el proceso de compilación, el rol de los archivos generados, la arquitectura de puertos Dual-Serve y la seguridad en la red interna.

---

## 1. El Flujo Completo: Visión General

```
 [1. Contrato Proto]              [2. Compilación Hermética]            [3. Código Generado Go]
 back/proto/<svc>/v1/*.proto  ──> ./generate.sh (Buf + Plugins)  ──>  back/<svc>/internal/gen/...
                                                                             │
 ┌───────────────────────────────────────────────────────────────────────────┴────────────────┐
 │                                                                                            │
 ▼ (Lado Cliente / Emisor)                                                                    ▼ (Lado Servidor / Receptor)
 Cliente gRPC inyecta metadata                                                       Interceptor valida x-internal-key
 (x-internal-key / Bearer)                                                           Server desempaqueta struct Go
        │                                                                                     ▲
        │  [4. Transporte HTTP/2 Binario en Red K8s (CoreDNS)]                                │
        └───────────────────────────> Service:Puerto (ej: notes:9082) ────────────────────────┘
```

El ciclo de vida se resume en 6 etapas:
1. **Diseño:** Se escribe el contrato agnóstico en archivos `.proto`.
2. **Compilación:** Se ejecuta `buf generate` $\rightarrow$ se producen archivos `.pb.go` y `_grpc.pb.go`.
3. **Servidor:** El microservicio receptor implementa la interfaz generada y abre un puerto TCP gRPC (modo Dual-Serve).
4. **Cliente:** El microservicio emisor importa los stubs Go y se conecta al nombre del servicio (resuelto por CoreDNS).
5. **Llamada:** El emisor llama al método como una función Go normal, adjuntando la cabecera `x-internal-key` para autenticación.
6. **Recepción:** El interceptor receptor valida la clave y despacha la lógica del servidor.

---

## 2. Paso a Paso con Ejemplos Reales del Proyecto

### Paso 1: El Contrato de Dominio (`.proto`)
**¿Para qué se usa?** Es la especificación neutral, fuertemente tipada y estricta de qué servicios, funciones, campos y mensajes existen.

* **Ejemplo Real (`academic.proto`):**  
  Ruta: `back/proto/infacadem/academic.proto`
  ```protobuf
  syntax = "proto3";
  package academic.v1;
  option go_package = "github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/gen/academicv1";

  service AcademicService {
    // GetSubject resuelve una materia solo si pertenece al solicitante (Zero-Knowledge)
    rpc GetSubject(GetSubjectRequest) returns (GetSubjectResponse);
  }

  message GetSubjectRequest {
    string subject_id = 1;
    string requester_user_id = 2;
  }

  message GetSubjectResponse {
    bool exists = 1;
    string subject_id = 2;
    string owner_user_id = 3;
    string name = 4;
    string status = 5;
  }
  ```

---

### Paso 2: Compilación Hermética con Buf
**¿Cómo se compila?** Para evitar diferencias entre versiones de `protoc` de cada máquina de desarrollo, se usa **Buf v1.59.0** con plugins oficiales de Go.

* Script: `back/proto/infacadem/generate.sh`
* Configuración: `back/proto/infacadem/buf.gen.yaml`
```yaml
version: v1
plugins:
  - plugin: go
    out: ../../infacadem/internal/gen/academicv1
    opt: paths=source_relative
  - plugin: go-grpc
    out: ../../infacadem/internal/gen/academicv1
    opt: paths=source_relative
```
Al ejecutar `./generate.sh`, Buf analiza la sintaxis (`buf lint`) y produce el código en Go.

---

### Paso 3: ¿Para qué sirven los compilados (`.pb.go`)?
La compilación genera dos archivos Go:
1. `academic.pb.go`: Structs nativos en Go con métodos de serialización binaria ultrarrápida.
2. `academic_grpc.pb.go`: Interfaces Go para el cliente y el servidor.

**Decisión de arquitectura:**  
Los archivos compilados **se commitean** directamente en carpetas como `back/infacadem/internal/gen/academicv1/` y `back/notes/internal/gen/notesv1/`. Esto se debe a que en el entorno de despliegue cada Dockerfile solo copia su propio microservicio; si los stubs estuvieran fuera en un paquete compartido en la raíz, los contenedores no podrían compilar de forma aislada.

---

### Paso 4: Arquitectura de Puertos: Dual-Serve (HTTP + gRPC)
Los microservicios **no transforman** HTTP a gRPC. Ambos servidores corren **en paralelo** dentro del mismo proceso `main.go`:
- Servidor **HTTP (Gin)** en puerto `808x`: atiende al Frontend Flutter a través del Nginx Proxy.
- Servidor **gRPC** en puerto `908x`: atiende llamadas entre microservicios por HTTP/2 binario.

| Servicio | HTTP | gRPC | Función Principal gRPC |
|---|:---:|:---:|---|
| **`auth`** | `8080` | `9080` | Entrega de OAuth tokens (Drive/Calendar) a otros servicios |
| **`infacadem`** | `8081` | `9081` | Validación de asignaturas (`GetSubject`) para Notes |
| **`notes`** | `8082` | `9082` | Limpieza de apuntes compartidos (`UnshareAll`) al salir de grupos |
| **`social`** | `8083` | `9083` | Verificación de membresías y correos de miembros de grupos |

**Implementación del Server (`infacadem/cmd/server/main.go`):**
```go
lis, _ := net.Listen("tcp", ":"+cfg.GRPCPort) // Puerto 9081
grpcSrv := grpc.NewServer()
academicv1.RegisterAcademicServiceServer(grpcSrv, 
    grpcHandler.NewAcademicServer(repository.NewSubjectRepository(pool)))
go grpcSrv.Serve(lis)
```

---

### Paso 5: Llamadas como Métodos Go y Resolución DNS
En el código de la aplicación, el microservicio cliente llama al servidor **exactamente como si fuera una función o método local de Go**:

Ruta: `back/social/internal/service/notes_grpc.go`
```go
// 1. Conexión usando el nombre del servicio Kubernetes (CoreDNS resuelve el IP del pod)
conn, _ := grpc.NewClient("notes:9082", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := notesv1.NewNotesServiceClient(conn)

// 2. Llamada directa por método Go:
resp, err := client.UnshareAll(ctx, &notesv1.UnshareAllRequest{
    UserId:  userID,
    GroupId: groupID,
})
```

---

### Paso 6: Seguridad y Autenticación con `x-internal-key`

> **Aclaración clave:** La comunicación interna dentro del clúster de Kubernetes se realiza en texto binario plano (`insecure.NewCredentials()`), ya que el tráfico vive en la red privada del clúster. La `x-internal-key` **no encripta el mensaje, sino que realiza autenticación y control de acceso**.

* **El cliente inyecta la clave en los metadatos gRPC:**
  ```go
  ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-key", s.apiKey)
  ```
* **El receptor valida la clave mediante un interceptor:**  
  Ruta: `back/notes/internal/interceptor/auth.go`
  ```go
  md, _ := metadata.FromIncomingContext(ctx)
  keys := md.Get("x-internal-key")
  if subtle.ConstantTimeCompare([]byte(keys[0]), []byte(expectedSecret)) != 1 {
      return nil, status.Error(codes.Unauthenticated, "invalid internal key")
  }
  ```
* **Degradación / Fallback:** Si la conexión gRPC fallara o la variable `SOCIAL_GRPC_ADDR` no estuviera presente, adaptadores como `SocialHTTPAdapter` (`back/notes/internal/service/social_adapter.go`) automáticamente conmutan al canal HTTP REST privado usando el header `X-Internal-Key`.
