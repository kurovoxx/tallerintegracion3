# Contrato gRPC de Academic (`academic.v1`)

`academic.proto` define `AcademicService.GetSubject`, que Notes usará para
validar `subject_id` (hoy UUID opaco). El server vive en
`back/infacadem/internal/handler/grpc` (puerto 9081 / `GRPC_PORT`).

## Regenerar (hermético, sin `protoc` del sistema)

```bash
cd back/proto/infacadem
./generate.sh
```

Fija el toolchain vía `go install`: buf v1.59.0, protoc-gen-go v1.36.12,
protoc-gen-go-grpc v1.5.1. La salida va a
`back/infacadem/internal/gen/academicv1` (commiteada, no editar a mano) y
correr dos veces deja diff vacío.

## Consumir desde Notes (ejemplo, sin implementar el cliente aquí)

```go
import (
    academicv1 "github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/gen/academicv1"
    "google.golang.org/grpc"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/status"
)

// En Pillán, "academic:9081" es el Service k8s (mismo namespace o FQDN).
conn, err := grpc.NewClient("academic:9081",
    grpc.WithTransportCredentials(insecure.NewCredentials()))
if err != nil { /* ... */ }
defer conn.Close()

client := academicv1.NewAcademicServiceClient(conn)
resp, err := client.GetSubject(ctx, &academicv1.GetSubjectRequest{
    SubjectId:       subjectID, // notes.notes.subject_id
    RequesterUserId: userID,    // del JWT validado en Notes
})
if err != nil {
    st, _ := status.FromError(err)
    switch st.Code() {
    case codes.NotFound:
        // No existe o no es del solicitante (indistinguibles a propósito).
    case codes.InvalidArgument:
        // subject_id o requester_user_id malformados.
    }
    return err
}
// err == nil ⇒ resp.GetExists() siempre true.
_ = resp.GetName()
```

Notas:

- `owner_user_id` es eco del solicitante (el server solo responde lo propio).
- `GET /api-auth/internal/` sigue bloqueado en nginx: Notes debe llamar al
  gRPC directo (`academic:9081`), nunca por el ingress HTTP.
