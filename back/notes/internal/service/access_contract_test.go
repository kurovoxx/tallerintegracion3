package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

func TestGetAccessTypedContract(t *testing.T) {
	for _, tc := range []struct {
		name, visibility, share, mode string
		owner                         bool
	}{
		{"private owner", "private", "", "owner", true},
		{"public reader", "public", "", "public", false},
		{"link reader", "private", "link", "link", false},
		{"restricted member", "private", "restricted", "restricted", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, notes, _, _, _, shared, social := newTestService()
			ctx := context.Background()
			owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			fileID := "access-contract-file"
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, tc.visibility, nil, "synced")
			if err != nil {
				t.Fatal(err)
			}
			if tc.share != "" {
				if _, err := shared.Create(ctx, note.ID, group, false, tc.share, 0); err != nil {
					t.Fatal(err)
				}
				if tc.share == "restricted" {
					social.AddMember(reader, group)
				}
			}
			if tc.owner {
				reader = owner
			}
			// Sin conexión OAuth no hay verificación Drive posible (el cliente
			// es nil): la bandera queda en false sin romper el endpoint local.
			svc.drive = nil
			var access *model.NoteAccessResponse
			access, err = svc.GetAccess(ctx, reader, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !access.CanRead || access.CanWrite != tc.owner || access.AccessMode != tc.mode || access.Visibility != tc.visibility {
				t.Fatalf("unexpected application access: %+v", access)
			}
			// Sin filas en drive_managed_permissions la convergencia local es 'synced'.
			if access.DriveSyncStatus != DriveSyncStatusSynced || access.DriveAccessVerified || access.DriveConnectionRequired {
				t.Fatalf("unexpected Drive metadata: %+v", access)
			}
			if access.DriveURL == nil || *access.DriveURL != "https://drive.google.com/file/d/"+fileID+"/view" {
				t.Fatalf("unexpected Drive URL: %v", access.DriveURL)
			}
		})
	}
}

// TestGetAccessDriveSyncStatusFromManagedPermissions fija el contrato de
// convergencia: el estado sale de notes.drive_managed_permissions (failed gana
// a pending, pending gana a in_sync) y el sync_status del archivo .md no
// influye en la respuesta.
func TestGetAccessDriveSyncStatusFromManagedPermissions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []string
		want     string
	}{
		{"sin permisos gestionados", nil, DriveSyncStatusSynced},
		{"todos in_sync", []string{model.PermissionSyncInSync, model.PermissionSyncInSync}, DriveSyncStatusSynced},
		{"un pending", []string{model.PermissionSyncInSync, model.PermissionSyncPending}, DriveSyncStatusPending},
		{"un failed", []string{model.PermissionSyncInSync, model.PermissionSyncFailed}, DriveSyncStatusFailed},
		{"failed gana a pending", []string{model.PermissionSyncPending, model.PermissionSyncFailed}, DriveSyncStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, notes, _, _, _, shared, _ := newTestService()
			ctx := context.Background()
			owner := uuid.NewString()
			fileID := "access-contract-file"
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, "private", nil, "failed_sync")
			if err != nil {
				t.Fatal(err)
			}
			for _, status := range tc.statuses {
				if _, err := shared.UpsertManagedPermission(ctx, &model.DriveManagedPermission{
					NoteID:         note.ID,
					ExternalFileID: fileID,
					PrincipalType:  model.PermissionPrincipalEmail,
					PrincipalKey:   uuid.NewString() + "@example.com",
					Role:           "reader",
					SyncStatus:     status,
				}); err != nil {
					t.Fatal(err)
				}
			}
			access, err := svc.GetAccess(ctx, owner, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if access.DriveSyncStatus != tc.want {
				t.Fatalf("DriveSyncStatus = %q, want %q", access.DriveSyncStatus, tc.want)
			}
			got, err := svc.GetAccessInfo(ctx, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("GetAccessInfo = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestGetAccessDriveConnectionRequiredOnlyForOwner fija la semántica estricta de
// la conexión OAuth: solo el autor de una nota no pública (access_mode=owner)
// depende de su token de Drive, así que es el único caso donde la respuesta
// declara DriveConnectionRequired=true. Para link y restricted el archivo se
// sirve con la autorización de la aplicación, de modo que la falta de conexión
// del lector no se reporta. El estado de convergencia derivado de
// notes.drive_managed_permissions no se altera en ningún caso.
func TestGetAccessDriveConnectionRequiredOnlyForOwner(t *testing.T) {
	for _, tc := range []struct {
		name, visibility, share, mode, permissionStatus, wantSync string
		owner, wantRequired                                       bool
	}{
		{"owner sin conexión", "private", "", "owner", model.PermissionSyncInSync, DriveSyncStatusSynced, true, true},
		{"owner de nota pública sin conexión", "public", "", "owner", model.PermissionSyncInSync, DriveSyncStatusSynced, true, false},
		{"link reader sin conexión con permisos pending", "private", "link", "link", model.PermissionSyncPending, DriveSyncStatusPending, false, false},
		{"restricted member sin conexión con permisos failed", "private", "restricted", "restricted", model.PermissionSyncFailed, DriveSyncStatusFailed, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, driveMock, notes, _, _, _, shared, social := newTestService()
			ctx := context.Background()
			owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			fileID := "access-contract-file"
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, tc.visibility, nil, "synced")
			if err != nil {
				t.Fatal(err)
			}
			if tc.share != "" {
				if _, err := shared.Create(ctx, note.ID, group, false, tc.share, 0); err != nil {
					t.Fatal(err)
				}
				if tc.share == "restricted" {
					social.AddMember(reader, group)
				}
			}
			if _, err := shared.UpsertManagedPermission(ctx, &model.DriveManagedPermission{
				NoteID:         note.ID,
				ExternalFileID: fileID,
				PrincipalType:  model.PermissionPrincipalEmail,
				PrincipalKey:   uuid.NewString() + "@example.com",
				Role:           "reader",
				SyncStatus:     tc.permissionStatus,
			}); err != nil {
				t.Fatal(err)
			}
			if tc.owner {
				reader = owner
			}
			driveMock.Disconnect(reader)
			access, err := svc.GetAccess(ctx, reader, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !access.CanRead || access.AccessMode != tc.mode {
				t.Fatalf("unexpected application access: %+v", access)
			}
			if access.DriveSyncStatus != tc.wantSync {
				t.Fatalf("DriveSyncStatus = %q, want %q", access.DriveSyncStatus, tc.wantSync)
			}
			if access.DriveConnectionRequired != tc.wantRequired {
				t.Fatalf("DriveConnectionRequired = %v, want %v: %+v", access.DriveConnectionRequired, tc.wantRequired, access)
			}
			if access.DriveAccessVerified {
				t.Fatalf("DriveAccessVerified = true sin verificación de Drive: %+v", access)
			}
		})
	}
}

// TestGetAccessDriveAccessVerified fija el contrato de verificación del autor:
// drive_access_verified=true solo cuando Drive confirma el acceso al archivo,
// y un 403/404 (permisos revocados o archivo ausente) deja la nota en false
// sin declarar conexión requerida.
func TestGetAccessDriveAccessVerified(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inDrive      bool
		getErr       *drive.DriveError
		wantVerified bool
	}{
		{"archivo propio accesible", true, nil, true},
		{"archivo ausente en Drive", false, nil, false},
		{"permisos revocados en Drive", true, &drive.DriveError{Code: 403, Message: "permiso revocado"}, false},
		{"fallo transitorio del upstream", true, &drive.DriveError{Code: 500, Message: "upstream caído"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, driveMock, notes, _, _, _, _, _ := newTestService()
			ctx := context.Background()
			owner := uuid.NewString()
			fileID := "access-contract-file"
			if tc.inDrive {
				created, err := driveMock.CreateFile(ctx, owner, "note-1", "contract.md", "contenido")
				if err != nil {
					t.Fatal(err)
				}
				fileID = created
			}
			if tc.getErr != nil {
				driveMock.InjectGetError(fileID, tc.getErr)
			}
			counter := &countingDriveClient{MockClient: driveMock}
			svc.drive = counter
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, "private", nil, "synced")
			if err != nil {
				t.Fatal(err)
			}
			access, err := svc.GetAccess(ctx, owner, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if access.AccessMode != "owner" {
				t.Fatalf("access_mode = %q, want owner", access.AccessMode)
			}
			if access.DriveAccessVerified != tc.wantVerified {
				t.Fatalf("DriveAccessVerified = %v, want %v: %+v", access.DriveAccessVerified, tc.wantVerified, access)
			}
			if access.DriveConnectionRequired {
				t.Fatalf("DriveConnectionRequired = true sin falta de OAuth: %+v", access)
			}
			if counter.calls != 1 {
				t.Fatalf("VerifyFileAccess llamado %d veces, want 1", counter.calls)
			}
		})
	}
}

// TestGetAccessDoesNotVerifyDriveForReaders fija que link, restricted y public
// no consultan Drive: el solicitante abre la nota con la autorización de la
// aplicación, así que VerifyFileAccess no debe invocarse ni siquiera cuando el
// lector no tiene conexión OAuth.
func TestGetAccessDoesNotVerifyDriveForReaders(t *testing.T) {
	for _, tc := range []struct {
		name, visibility, share, mode string
	}{
		{"link reader", "private", "link", "link"},
		{"restricted member", "private", "restricted", "restricted"},
		{"public reader", "public", "", "public"},
		{"owner de nota pública", "public", "", "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, driveMock, notes, _, _, _, shared, social := newTestService()
			ctx := context.Background()
			owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			fileID := "access-contract-file"
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, tc.visibility, nil, "synced")
			if err != nil {
				t.Fatal(err)
			}
			if tc.share != "" {
				if _, err := shared.Create(ctx, note.ID, group, false, tc.share, 0); err != nil {
					t.Fatal(err)
				}
				if tc.share == "restricted" {
					social.AddMember(reader, group)
				}
			}
			if tc.mode == "owner" {
				reader = owner
			}
			counter := &countingDriveClient{MockClient: driveMock}
			svc.drive = counter
			// Sin conexión OAuth: si se consultara Drive, la respuesta la
			// declararía con DriveConnectionRequired=true.
			driveMock.Disconnect(reader)
			access, err := svc.GetAccess(ctx, reader, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if access.AccessMode != tc.mode {
				t.Fatalf("access_mode = %q, want %q", access.AccessMode, tc.mode)
			}
			if access.DriveAccessVerified || access.DriveConnectionRequired {
				t.Fatalf("banderas de Drive inesperadas: %+v", access)
			}
			if counter.calls != 0 {
				t.Fatalf("VerifyFileAccess llamado %d veces, want 0", counter.calls)
			}
		})
	}
}

// TestGetAccessDriveConnectionRequiredOnlyForOAuth fija la semántica fina: la
// bandera solo se activa por falta de conexión OAuth, no por un 403 de permisos
// de Drive (p. ej. un lector link cuyo archivo aún no está compartido con su
// cuenta) ni por una nota ausente en Drive.
func TestGetAccessDriveConnectionRequiredOnlyForOAuth(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inDrive bool
	}{
		{"lector con conexión pero sin permiso en Drive", true},
		{"lector con conexión y archivo ausente en Drive", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, driveMock, notes, _, _, _, shared, _ := newTestService()
			ctx := context.Background()
			owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
			fileID := "access-contract-missing"
			if tc.inDrive {
				created, err := driveMock.CreateFile(ctx, owner, "note-1", "contract.md", "contenido")
				if err != nil {
					t.Fatal(err)
				}
				fileID = created
			}
			note, err := notes.Create(ctx, "", owner, nil, "contract", &fileID, "private", nil, "synced")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := shared.Create(ctx, note.ID, group, false, "link", 0); err != nil {
				t.Fatal(err)
			}
			access, err := svc.GetAccess(ctx, reader, note.ID)
			if err != nil {
				t.Fatal(err)
			}
			if access.AccessMode != "link" {
				t.Fatalf("access_mode = %q, want link", access.AccessMode)
			}
			if access.DriveConnectionRequired {
				t.Fatalf("DriveConnectionRequired = true sin falta de OAuth: %+v", access)
			}
		})
	}
}

// countingDriveClient cuenta las llamadas a VerifyFileAccess para fijar que el
// endpoint /access sólo consulta Drive cuando la semántica lo exige.
type countingDriveClient struct {
	*drive.MockClient
	calls int
}

func (c *countingDriveClient) VerifyFileAccess(ctx context.Context, userID string, fileID string) error {
	c.calls++
	return c.MockClient.VerifyFileAccess(ctx, userID, fileID)
}
