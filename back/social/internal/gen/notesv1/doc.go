// Package notesv1 contiene una copia local, generada, de los stubs Go del
// contrato gRPC de Notes (back/proto/notes/v1/notes.proto).
//
// Por qué una copia: los microservicios de este monorepo NO dependen entre sí
// (ver documentation/docMartin.md §3_5_3); además, los stubs de Notes viven bajo
// back/notes/internal/..., que Go prohíbe importar desde otro módulo
// (back/social es un módulo distinto). Social solo necesita el CLIENTE
// (NotesServiceClient) para llamar a UnshareAll.
//
// No editar a mano. Para refrescar tras un cambio del contrato:
//  1. cd back/proto/notes && ./generate.sh   (regenera los stubs de Notes)
//  2. copiar back/notes/internal/gen/notesv1/v1/notes.pb.go y notes_grpc.pb.go
//     sobre este directorio (mismos nombres).
//
// El paquete generado se llama notesv1 (viene del option go_package), por lo
// que el import path de Social es
// github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/notesv1.
package notesv1
