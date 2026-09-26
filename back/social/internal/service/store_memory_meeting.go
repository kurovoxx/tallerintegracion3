package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// MemoryMeetingStore es una implementación en memoria de MeetingRepo para tests.
// Replica lo observable del repository real: grupo inexistente → pgx.ErrNoRows,
// membresía por conjunto, reunión generada con id/timestamps, y notificaciones
// in-app contadas (sin persistir filas, solo cantidad por reunión).
type MemoryMeetingStore struct {
	mu         sync.Mutex
	groups     map[string]bool
	members    map[string]map[string]bool   // grupo -> conjunto de usuarios
	roles      map[string]map[string]string // grupo -> usuario -> rol
	meetings   map[string]sqlc.SocialMeeting
	byGroup    map[string][]string // grupo -> ids de reuniones en orden
	notifySeen map[string]int      // meeting_id -> notificaciones generadas
	calEvents  map[string]string   // meeting_id -> google_calendar_event_id persistido
	discordCfg map[string]sqlc.SocialDiscordIntegration
	streamChan map[string]string // grupo -> channel_id de Stream registrado
}

func NewMemoryMeetingStore() *MemoryMeetingStore {
	return &MemoryMeetingStore{
		groups:     map[string]bool{},
		members:    map[string]map[string]bool{},
		roles:      map[string]map[string]string{},
		meetings:   map[string]sqlc.SocialMeeting{},
		byGroup:    map[string][]string{},
		notifySeen: map[string]int{},
		calEvents:  map[string]string{},
		discordCfg: map[string]sqlc.SocialDiscordIntegration{},
		streamChan: map[string]string{},
	}
}

// AddGroup registra un grupo con sus miembros para armar escenarios.
// El primer miembro queda como admin, el resto como member.
func (m *MemoryMeetingStore) AddGroup(groupID string, memberIDs ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[groupID] = true
	set := map[string]bool{}
	roles := map[string]string{}
	for i, u := range memberIDs {
		set[u] = true
		if i == 0 {
			roles[u] = "admin"
		} else {
			roles[u] = "member"
		}
	}
	m.members[groupID] = set
	m.roles[groupID] = roles
}

// NotificationsFor cuenta las notificaciones generadas para una reunión.
func (m *MemoryMeetingStore) NotificationsFor(meetingID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifySeen[meetingID]
}

// CalendarEventFor retorna el google_calendar_event_id persistido (""
// si el sync no llegó o falló).
func (m *MemoryMeetingStore) CalendarEventFor(meetingID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calEvents[meetingID]
}

// SetCalendarEventID persiste el event id del sync (lo usa CalendarMeetingNotifier).
func (m *MemoryMeetingStore) SetCalendarEventID(_ context.Context, meetingID pgtype.UUID, eventID string) (sqlc.SocialMeeting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mid, err := uuid.FromBytes(meetingID.Bytes[:])
	if err != nil || !meetingID.Valid {
		return sqlc.SocialMeeting{}, pgx.ErrNoRows
	}
	meeting, ok := m.meetings[mid.String()]
	if !ok {
		return sqlc.SocialMeeting{}, pgx.ErrNoRows
	}
	var ev pgtype.Text
	if err := ev.Scan(eventID); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	meeting.GoogleCalendarEventID = ev
	m.meetings[mid.String()] = meeting
	m.calEvents[mid.String()] = eventID
	return meeting, nil
}

func mustUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic(err)
	}
	return u
}

func (m *MemoryMeetingStore) GetGroupByID(_ context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(id.Bytes[:])
	if err != nil || !id.Valid {
		return sqlc.SocialGroup{}, pgx.ErrNoRows
	}
	if !m.groups[gid.String()] {
		return sqlc.SocialGroup{}, pgx.ErrNoRows
	}
	return sqlc.SocialGroup{ID: id}, nil
}

func (m *MemoryMeetingStore) IsMember(_ context.Context, groupID, userID pgtype.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err1 := uuid.FromBytes(groupID.Bytes[:])
	uid, err2 := uuid.FromBytes(userID.Bytes[:])
	if err1 != nil || err2 != nil || !groupID.Valid || !userID.Valid {
		return false, nil
	}
	return m.members[gid.String()][uid.String()], nil
}

func (m *MemoryMeetingStore) CreateMeetingWithNotifications(_ context.Context, arg sqlc.CreateMeetingParams) (sqlc.SocialMeeting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(arg.GroupID.Bytes[:])
	if err != nil || !m.groups[gid.String()] {
		return sqlc.SocialMeeting{}, pgx.ErrNoRows
	}
	var mid pgtype.UUID
	if err := mid.Scan(uuid.NewString()); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	now := time.Now().UTC()
	meeting := sqlc.SocialMeeting{
		ID:              mid,
		GroupID:         arg.GroupID,
		Title:           arg.Title,
		Description:     arg.Description,
		ScheduledAt:     arg.ScheduledAt,
		CreatedByUserID: arg.CreatedByUserID,
		NotifyDiscord:   arg.NotifyDiscord,
		CreatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
	}
	midStr, _ := uuid.FromBytes(mid.Bytes[:])
	m.meetings[midStr.String()] = meeting
	m.byGroup[gid.String()] = append(m.byGroup[gid.String()], midStr.String())
	// Notificaciones in-app: un miembro = una fila, excepto el creador.
	creator, _ := uuid.FromBytes(arg.CreatedByUserID.Bytes[:])
	count := 0
	for u := range m.members[gid.String()] {
		if u != creator.String() {
			count++
		}
	}
	m.notifySeen[midStr.String()] = count
	return meeting, nil
}

func (m *MemoryMeetingStore) ListMeetingsByGroup(_ context.Context, groupID pgtype.UUID) ([]sqlc.SocialMeeting, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(groupID.Bytes[:])
	if err != nil || !groupID.Valid {
		return nil, nil
	}
	out := make([]sqlc.SocialMeeting, 0, len(m.byGroup[gid.String()]))
	for _, mid := range m.byGroup[gid.String()] {
		out = append(out, m.meetings[mid])
	}
	return out, nil
}

var _ MeetingRepo = (*MemoryMeetingStore)(nil)

// GetMemberRole responde el rol o pgx.ErrNoRows si no es miembro (para DiscordRepo).
func (m *MemoryMeetingStore) GetMemberRole(_ context.Context, groupID, userID pgtype.UUID) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err1 := uuid.FromBytes(groupID.Bytes[:])
	uid, err2 := uuid.FromBytes(userID.Bytes[:])
	if err1 != nil || err2 != nil {
		return "", pgx.ErrNoRows
	}
	role, ok := m.roles[gid.String()][uid.String()]
	if !ok {
		return "", pgx.ErrNoRows
	}
	return role, nil
}

// UpsertConfig crea o reemplaza la config de Discord del grupo.
func (m *MemoryMeetingStore) UpsertConfig(_ context.Context, arg sqlc.UpsertDiscordConfigParams) (sqlc.SocialDiscordIntegration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(arg.GroupID.Bytes[:])
	if err != nil || !arg.GroupID.Valid {
		return sqlc.SocialDiscordIntegration{}, pgx.ErrNoRows
	}
	cfg := sqlc.SocialDiscordIntegration{
		GroupID:    arg.GroupID,
		ServerName: arg.ServerName,
		InviteUrl:  arg.InviteUrl,
		WebhookUrl: arg.WebhookUrl,
	}
	m.discordCfg[gid.String()] = cfg
	return cfg, nil
}

// GetConfigByGroup responde la config o pgx.ErrNoRows (para MeetingDiscordStore).
func (m *MemoryMeetingStore) GetConfigByGroup(_ context.Context, groupID pgtype.UUID) (sqlc.SocialDiscordIntegration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(groupID.Bytes[:])
	if err != nil || !groupID.Valid {
		return sqlc.SocialDiscordIntegration{}, pgx.ErrNoRows
	}
	cfg, ok := m.discordCfg[gid.String()]
	if !ok {
		return sqlc.SocialDiscordIntegration{}, pgx.ErrNoRows
	}
	return cfg, nil
}

var _ DiscordRepo = (*MemoryMeetingStore)(nil)
var _ MeetingDiscordStore = (*MemoryMeetingStore)(nil)

// SetStreamChannel registra el canal de Stream del grupo (setup de tests).
func (m *MemoryMeetingStore) SetStreamChannel(groupID, channelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.streamChan[groupID] = channelID
}

// GetChannelByGroup responde el canal o pgx.ErrNoRows (para StreamMessageStore).
func (m *MemoryMeetingStore) GetChannelByGroup(_ context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	gid, err := uuid.FromBytes(groupID.Bytes[:])
	if err != nil || !groupID.Valid {
		return sqlc.SocialStreamChannel{}, pgx.ErrNoRows
	}
	ch, ok := m.streamChan[gid.String()]
	if !ok || ch == "" {
		return sqlc.SocialStreamChannel{}, pgx.ErrNoRows
	}
	return sqlc.SocialStreamChannel{GroupID: groupID, ChannelID: ch}, nil
}

var _ StreamMessageStore = (*MemoryMeetingStore)(nil)
