//go:build crossservice

package test

import (
	"context"
	"strings"
	"testing"
	"time"

	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Complementa el ciclo de membresía con todos los endpoints de Social que no
// participan del share: usa el servidor, el JWT de Identity y Postgres reales.
func testSocialRoutes(t *testing.T, s *stack) {
	owner := s.register(t, "Responsable de rutas")
	member := s.register(t, "Integrante de rutas")
	outsider := s.register(t, "Externo de rutas")

	t.Run("health_and_group_administration", func(t *testing.T) {
		var health struct{ Status, Service string }
		s.request(t, "GET", s.social+"/health", "", nil, 200, &health)
		if health.Status != "UP" || health.Service != "social-service" {
			t.Fatalf("health: %+v", health)
		}
		var database struct{ Status, Database string }
		s.request(t, "GET", s.social+"/health/db", "", nil, 200, &database)
		if database.Status != "UP" || !strings.HasPrefix(database.Database, "social_cross_e2e_") {
			t.Fatalf("health/db no usa la BD aislada: %+v", database)
		}
		s.request(t, "GET", s.social+"/groups/me", "", nil, 401, nil)
		group, oldInvite := s.group(t, owner)
		base := s.social + "/groups/" + group
		s.join(t, member, group, oldInvite, 201)
		var mine []struct {
			ID   string `json:"group_id"`
			Role string `json:"role"`
		}
		s.request(t, "GET", s.social+"/groups/me", member.token, nil, 200, &mine)
		if len(mine) != 1 || mine[0].ID != group || mine[0].Role != "member" {
			t.Fatalf("mis grupos: %+v", mine)
		}
		s.request(t, "POST", base+"/invite/regenerate", member.token, nil, 403, nil)
		var invite struct {
			Token string `json:"new_invite_token"`
		}
		s.request(t, "POST", base+"/invite/regenerate", owner.token, nil, 200, &invite)
		if invite.Token == "" || invite.Token == oldInvite {
			t.Fatalf("invitación sin rotar: %+v", invite)
		}
		s.join(t, outsider, group, oldInvite, 404)
		s.join(t, outsider, group, invite.Token, 201)
		roleURL := base + "/members/" + member.id + "/role"
		s.request(t, "PATCH", base+"/members/"+owner.id+"/role", member.token, map[string]string{"role": "member"}, 403, nil)
		s.request(t, "PATCH", roleURL, owner.token, map[string]string{"role": "admin"}, 200, nil)
		var view struct {
			Role   string `json:"role"`
			Invite string `json:"invite_token"`
		}
		s.request(t, "GET", base, member.token, nil, 200, &view)
		if view.Role != "admin" || view.Invite != invite.Token {
			t.Fatalf("promoción no persistida: %+v", view)
		}
		s.request(t, "PATCH", roleURL, owner.token, map[string]string{"role": "member"}, 200, nil)
		s.request(t, "POST", base+"/transfer-admin", owner.token, map[string]string{"new_admin_user_id": member.id}, 200, nil)
		view = struct {
			Role   string `json:"role"`
			Invite string `json:"invite_token"`
		}{}
		s.request(t, "GET", base, owner.token, nil, 200, &view)
		if view.Role != "member" || view.Invite != "" {
			t.Fatalf("antiguo administrador conserva privilegios: %+v", view)
		}
		s.request(t, "POST", base+"/invite/regenerate", owner.token, nil, 403, nil)

		// Estos métodos son los mismos que consumen Notes e Identity por gRPC.
		ctx, cancel := rpcContext()
		isMember, err := s.rpc.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: group, UserId: owner.id})
		cancel()
		if err != nil || !isMember.GetIsMember() || isMember.GetRole() != socialv1.Role_ROLE_MEMBER {
			t.Fatalf("IsMember tras transferencia: %v, %v", isMember, err)
		}
		ctx, cancel = rpcContext()
		isAdmin, err := s.rpc.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: group, UserId: member.id})
		cancel()
		if err != nil || !isAdmin.GetIsAdmin() {
			t.Fatalf("IsAdmin sucesor: %v, %v", isAdmin, err)
		}
		ctx, cancel = rpcContext()
		oldAdmin, err := s.rpc.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: group, UserId: owner.id})
		cancel()
		if err != nil || oldAdmin.GetIsAdmin() {
			t.Fatalf("IsAdmin anterior: %v, %v", oldAdmin, err)
		}
		ctx, cancel = rpcContext()
		members, err := s.rpc.GetMembers(ctx, &socialv1.GetMembersRequest{GroupId: group})
		cancel()
		if err != nil || len(members.GetMembers()) != 3 {
			t.Fatalf("GetMembers: %v, %v", members, err)
		}
		found := false
		for _, m := range members.GetMembers() {
			if m.GetUserId() == member.id {
				found = m.GetRole() == socialv1.Role_ROLE_ADMIN && m.GetEmail() == member.email && m.GetDisplayName() == member.name
			}
		}
		if !found {
			t.Fatalf("GetMembers perdió rol o identidad del sucesor: %v", members)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		_, err = s.rpc.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: group, UserId: owner.id})
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("IsMember sin clave: %v", err)
		}

		internal := s.social + "/internal/groups/" + group
		headers := map[string]string{"X-Internal-Key": internalKey}
		var internalMembers []struct {
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		}
		for _, path := range []string{"/members", "/member-emails"} {
			s.request(t, "GET", internal+path, owner.token, nil, 401, nil)
			s.request(t, "GET", internal+path, "", nil, 401, nil, map[string]string{"X-Internal-Key": "incorrecta"})
		}
		s.request(t, "GET", internal+"/members", "", nil, 200, &internalMembers, headers)
		roles := map[string]string{}
		for _, m := range internalMembers {
			roles[m.UserID] = m.Role
		}
		if len(roles) != 3 || roles[member.id] != "admin" || roles[owner.id] != "member" {
			t.Fatalf("miembros internos: %+v", internalMembers)
		}
		var emails []string
		s.request(t, "GET", internal+"/member-emails", "", nil, 200, &emails, headers)
		if len(emails) != 3 || !containsString(emails, owner.email) || !containsString(emails, member.email) || !containsString(emails, outsider.email) {
			t.Fatalf("correos internos: %v", emails)
		}
	})

	t.Run("todo_sprint_hours_and_workspace", func(t *testing.T) {
		group, invite := s.group(t, owner)
		s.join(t, member, group, invite, 201)
		other, _ := s.group(t, owner)
		base := s.social + "/groups/" + group
		otherBase := s.social + "/groups/" + other
		type item struct {
			ID             string  `json:"id"`
			Title          string  `json:"title"`
			Name           string  `json:"name"`
			Status         string  `json:"status"`
			AssignedTo     string  `json:"assigned_to"`
			SheetID        string  `json:"sheet_id"`
			Priority       string  `json:"priority"`
			EstimatedHours float64 `json:"estimated_hours"`
			PeriodEnd      string  `json:"period_end"`
		}
		var todo struct{ Data item }
		todoBody := map[string]string{"title": "Preparar pruebas", "assigned_to": member.id, "status": "todo", "due_date": "2026-10-25"}
		s.request(t, "POST", base+"/todo", member.token, todoBody, 201, &todo)
		if todo.Data.ID == "" || todo.Data.AssignedTo != member.id || todo.Data.Status != "todo" {
			t.Fatalf("TODO creado: %+v", todo)
		}
		todoURL := base + "/todo/" + todo.Data.ID
		s.request(t, "PATCH", todoURL, member.token, map[string]string{"title": "Pruebas listas", "status": "done"}, 200, &todo)
		var todos struct{ Data []item }
		s.request(t, "GET", base+"/todo?status=done", member.token, nil, 200, &todos)
		if len(todos.Data) != 1 || todos.Data[0].ID != todo.Data.ID || todos.Data[0].Title != "Pruebas listas" {
			t.Fatalf("TODO editado/listado: %+v", todos)
		}
		s.request(t, "PATCH", otherBase+"/todo/"+todo.Data.ID, owner.token, map[string]string{"title": "Cruce inválido"}, 404, nil)
		s.request(t, "DELETE", otherBase+"/todo/"+todo.Data.ID, owner.token, nil, 404, nil)
		s.request(t, "POST", base+"/todo", member.token, map[string]string{"title": "Asignación inválida", "assigned_to": outsider.id}, 400, nil)

		var sheet struct{ Data item }
		s.request(t, "POST", base+"/sprint-sheets", owner.token, map[string]string{"name": "Sprint rutas", "period_start": "2026-10-19", "period_end": "2026-10-25"}, 201, &sheet)
		sheetID := sheet.Data.ID
		if sheetID == "" {
			t.Fatal("hoja sin ID")
		}
		s.request(t, "POST", base+"/sprint-sheets", member.token, map[string]string{"name": "Sprint siguiente", "period_start": "2026-10-26", "period_end": "2026-11-01"}, 201, nil)
		s.request(t, "PATCH", base+"/sprint-sheets/"+sheetID, member.token, map[string]string{"name": "Sprint actualizado", "period_end": "2026-10-26"}, 200, &sheet)
		var sheets struct{ Data []item }
		s.request(t, "GET", base+"/sprint-sheets", member.token, nil, 200, &sheets)
		found := false
		for _, sh := range sheets.Data {
			if sh.ID == sheetID {
				found = sh.Name == "Sprint actualizado" && sh.PeriodEnd == "2026-10-26"
			}
		}
		if !found {
			t.Fatalf("hoja editada no persistida: %+v", sheets)
		}
		s.request(t, "PATCH", otherBase+"/sprint-sheets/"+sheetID, owner.token, map[string]string{"name": "Cruce inválido"}, 404, nil)
		s.request(t, "DELETE", otherBase+"/sprint-sheets/"+sheetID, owner.token, nil, 404, nil)

		var task struct{ Data item }
		taskBody := map[string]any{"title": "Ejecutar E2E", "assigned_to": member.id, "sheet_id": sheetID, "priority": "media", "status": "sin_empezar", "estimated_hours": 4.5}
		s.request(t, "POST", base+"/sprint-sheet", member.token, taskBody, 201, &task)
		taskID := task.Data.ID
		if taskID == "" || task.Data.SheetID != sheetID || task.Data.AssignedTo != member.id {
			t.Fatalf("tarea sprint creada: %+v", task)
		}
		taskURL := base + "/sprint-sheet/" + taskID
		s.request(t, "PATCH", taskURL, member.token, map[string]any{"title": "E2E en curso", "status": "en_proceso", "priority": "alta", "estimated_hours": 6.5}, 200, &task)
		var tasks struct{ Data []item }
		s.request(t, "GET", base+"/sprint-sheet?sheet_id="+sheetID+"&status=en_proceso&priority=alta", owner.token, nil, 200, &tasks)
		if len(tasks.Data) != 1 || tasks.Data[0].ID != taskID || tasks.Data[0].Title != "E2E en curso" || tasks.Data[0].EstimatedHours != 6.5 {
			t.Fatalf("tarea sprint editada/listada: %+v", tasks)
		}
		s.request(t, "PATCH", otherBase+"/sprint-sheet/"+taskID, owner.token, map[string]string{"title": "Cruce inválido"}, 404, nil)
		s.request(t, "DELETE", otherBase+"/sprint-sheet/"+taskID, owner.token, nil, 404, nil)
		hoursURL := s.social + "/sprint-sheet/" + taskID + "/hours"
		var hours struct {
			Data []struct {
				ID    string  `json:"id"`
				Hours float64 `json:"hours"`
			}
			Total float64 `json:"total_hours"`
		}
		s.request(t, "POST", hoursURL, member.token, map[string]any{"log_date": "2026-10-19", "hours": 2.5}, 201, nil)
		s.request(t, "POST", hoursURL, member.token, map[string]any{"log_date": "2026-10-19", "hours": 3.5}, 200, nil)
		s.request(t, "POST", hoursURL, owner.token, map[string]any{"log_date": "2026-10-20", "hours": 1.5}, 201, nil)
		s.request(t, "GET", hoursURL, member.token, nil, 200, &hours)
		if len(hours.Data) != 2 || hours.Total != 5 {
			t.Fatalf("upsert duplicó horas o total incorrecto: %+v", hours)
		}
		s.request(t, "GET", hoursURL+"?from=2026-10-19&to=2026-10-19", owner.token, nil, 200, &hours)
		if len(hours.Data) != 1 || hours.Total != 3.5 || hours.Data[0].Hours != 3.5 {
			t.Fatalf("filtro de horas: %+v", hours)
		}

		for _, route := range []struct {
			method, path string
			body         any
		}{
			{"GET", base + "/todo", nil}, {"POST", base + "/todo", todoBody},
			{"PATCH", todoURL, map[string]string{"status": "todo"}}, {"DELETE", todoURL, nil},
			{"GET", base + "/sprint-sheets", nil}, {"POST", base + "/sprint-sheets", map[string]string{"name": "Ajeno", "period_start": "2026-10-19", "period_end": "2026-10-25"}},
			{"PATCH", base + "/sprint-sheets/" + sheetID, map[string]string{"name": "Ajeno"}}, {"DELETE", base + "/sprint-sheets/" + sheetID, nil},
			{"GET", base + "/sprint-sheet", nil}, {"POST", base + "/sprint-sheet", taskBody},
			{"PATCH", taskURL, map[string]string{"status": "listo"}}, {"DELETE", taskURL, nil},
			{"GET", hoursURL, nil}, {"POST", hoursURL, map[string]any{"log_date": "2026-10-19", "hours": 9}},
			{"GET", base + "/workspace", nil},
		} {
			s.request(t, route.method, route.path, outsider.token, route.body, 403, nil)
		}
		var workspace model.Workspace
		s.request(t, "GET", base+"/workspace", member.token, nil, 200, &workspace)
		if workspace.Group.ID != group || workspace.Group.Role != "member" || len(workspace.Kanban.Done) != 1 || workspace.Kanban.Done[0]["id"] != todo.Data.ID || len(workspace.SprintSheet.Tasks) != 1 || workspace.SprintSheet.Tasks[0]["id"] != taskID || workspace.Chat.TokenEndpoint != "/groups/"+group+"/stream-token" {
			t.Fatalf("workspace no refleja los recursos reales: %+v", workspace)
		}
		var overview model.MyOverview
		s.request(t, "GET", s.social+"/me/overview", member.token, nil, 200, &overview)
		found = false
		for _, card := range overview.Groups {
			if card.GroupID == group {
				found = card.Role == "member" && card.MemberCount == 2
			}
		}
		if overview.UserID != member.id || overview.Stats.GroupsCount != len(overview.Groups) || !found {
			t.Fatalf("overview: %+v", overview)
		}

		s.request(t, "DELETE", todoURL, member.token, nil, 200, nil)
		s.request(t, "GET", base+"/todo", member.token, nil, 200, &todos)
		if len(todos.Data) != 0 {
			t.Fatalf("TODO sigue listado después del delete: %+v", todos)
		}
		s.request(t, "DELETE", taskURL, member.token, nil, 200, nil)
		s.request(t, "GET", base+"/sprint-sheet", member.token, nil, 200, &tasks)
		if len(tasks.Data) != 0 {
			t.Fatalf("tarea sprint sigue listada después del delete: %+v", tasks)
		}
		s.request(t, "GET", hoursURL, member.token, nil, 404, nil)
		s.request(t, "DELETE", base+"/sprint-sheets/"+sheetID, member.token, nil, 200, nil)
		s.request(t, "GET", base+"/sprint-sheets", member.token, nil, 200, &sheets)
		for _, sh := range sheets.Data {
			if sh.ID == sheetID {
				t.Fatal("hoja eliminada sigue listada")
			}
		}
	})

	t.Run("meetings_notifications_discord_and_stream", func(t *testing.T) {
		group, invite := s.group(t, owner)
		s.join(t, member, group, invite, 201)
		base := s.social + "/groups/" + group
		discordBody := map[string]string{"server_name": "Grupo E2E", "invite_url": "https://discord.gg/academic-e2e"}
		s.request(t, "PUT", base+"/discord-config", member.token, discordBody, 403, nil)
		s.request(t, "PUT", base+"/discord-config", owner.token, discordBody, 200, nil)
		var discord struct {
			Server  string  `json:"server_name"`
			Invite  string  `json:"invite_url"`
			Webhook *string `json:"webhook_url"`
		}
		s.request(t, "GET", base+"/discord-config", member.token, nil, 200, &discord)
		if discord.Server != discordBody["server_name"] || discord.Invite != discordBody["invite_url"] || discord.Webhook != nil {
			t.Fatalf("configuración Discord sin webhook: %+v", discord)
		}
		var stream struct {
			Token   string `json:"token"`
			Channel string `json:"channel_id"`
		}
		s.request(t, "GET", base+"/stream-token", member.token, nil, 200, &stream)
		if stream.Token == "" || stream.Channel == "" {
			t.Fatalf("Stream no emitió token/canal: %+v", stream)
		}
		meetingBody := map[string]any{"title": "Revisión E2E", "description": "Persistencia y avisos", "scheduled_at": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339), "notify_discord": false}
		var meeting struct {
			ID string `json:"meeting_id"`
		}
		s.request(t, "POST", base+"/meetings", member.token, meetingBody, 201, &meeting)
		var meetings struct {
			Data []struct{ ID, Title string }
		}
		s.request(t, "GET", base+"/meetings", owner.token, nil, 200, &meetings)
		if meeting.ID == "" || len(meetings.Data) != 1 || meetings.Data[0].ID != meeting.ID || meetings.Data[0].Title != meetingBody["title"] {
			t.Fatalf("reunión creada/listada: %+v, %+v", meeting, meetings)
		}
		for _, route := range []struct {
			method, path string
			body         any
		}{
			{"POST", "/meetings", meetingBody}, {"GET", "/meetings", nil},
			{"GET", "/discord-config", nil}, {"PUT", "/discord-config", discordBody}, {"GET", "/stream-token", nil},
		} {
			s.request(t, route.method, base+route.path, outsider.token, route.body, 403, nil)
		}
		var notifications struct {
			Data []struct {
				ID        string  `json:"id"`
				MeetingID string  `json:"meeting_id"`
				Read      bool    `json:"read"`
				ReadAt    *string `json:"read_at"`
			}
		}
		s.request(t, "GET", s.social+"/notifications/me", owner.token, nil, 200, &notifications)
		if len(notifications.Data) != 1 || notifications.Data[0].MeetingID != meeting.ID || notifications.Data[0].Read {
			t.Fatalf("fan-out de notificaciones: %+v", notifications)
		}
		notificationID := notifications.Data[0].ID
		s.request(t, "POST", s.social+"/notifications/"+notificationID+"/read", outsider.token, nil, 404, nil)
		s.request(t, "POST", s.social+"/notifications/"+notificationID+"/read", member.token, nil, 404, nil)
		s.request(t, "POST", s.social+"/notifications/"+notificationID+"/read", owner.token, nil, 200, nil)
		s.request(t, "POST", s.social+"/notifications/"+notificationID+"/read", owner.token, nil, 404, nil)
		s.request(t, "GET", s.social+"/notifications/me", owner.token, nil, 200, &notifications)
		if len(notifications.Data) != 1 || !notifications.Data[0].Read || notifications.Data[0].ReadAt == nil {
			t.Fatalf("lectura no persistida: %+v", notifications)
		}
		s.request(t, "GET", s.social+"/notifications/me", member.token, nil, 200, &notifications)
		if len(notifications.Data) != 0 {
			t.Fatalf("se notificó al creador de la reunión (regla 4.8): %+v", notifications)
		}
		s.request(t, "GET", s.social+"/notifications/me", outsider.token, nil, 200, &notifications)
		if len(notifications.Data) != 0 {
			t.Fatalf("se filtraron avisos al externo: %+v", notifications)
		}
		var workspace model.Workspace
		s.request(t, "GET", base+"/workspace", owner.token, nil, 200, &workspace)
		if len(workspace.Meetings.Upcoming) != 1 || workspace.Meetings.Upcoming[0].ID != meeting.ID {
			t.Fatalf("workspace omite reunión: %+v", workspace.Meetings)
		}
	})
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
