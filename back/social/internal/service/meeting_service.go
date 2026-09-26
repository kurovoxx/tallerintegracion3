package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrMeetingNotFound     = errors.New("meeting not found")
	ErrScheduledAtRequired = errors.New("scheduled_at is required")
	ErrInvalidScheduledAt  = errors.New("invalid scheduled_at: must be RFC3339 (e.g. 2026-09-20T15:00:00Z)")
)

// MeetingCreatedNotifier dispara integraciones best-effort al agendar una reunión
// (Google Calendar, Discord, Stream). Un fallo aquí nunca revierte la reunión
// ya persistida — ver agentMain.md §2.4 y agentApiContract.md §5.
type MeetingCreatedNotifier interface {
	OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting)
}

type NoopMeetingCreatedNotifier struct{}

func (NoopMeetingCreatedNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {}

// MeetingRepo es la porción de persistencia que usa MeetingService.
// *repository.MeetingRepository es la implementación real (Postgres);
// MemoryMeetingStore, la de tests. La costura permite probar el servicio
// y los flujos con notifiers sin base de datos.
type MeetingRepo interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error)
	CreateMeetingWithNotifications(ctx context.Context, arg sqlc.CreateMeetingParams) (sqlc.SocialMeeting, error)
	ListMeetingsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialMeeting, error)
}

type MeetingService struct {
	repo     MeetingRepo
	notifier MeetingCreatedNotifier
}

func NewMeetingService(repo MeetingRepo, notifier MeetingCreatedNotifier) *MeetingService {
	if notifier == nil {
		notifier = NoopMeetingCreatedNotifier{}
	}
	return &MeetingService{repo: repo, notifier: notifier}
}

func parseMeetingUserUUID(userID string) (pgtype.UUID, error) {
	var uid pgtype.UUID
	if _, err := uuid.Parse(strings.TrimSpace(userID)); err != nil {
		return uid, ErrInvalidUserID
	}
	if err := uid.Scan(strings.TrimSpace(userID)); err != nil {
		return uid, ErrInvalidUserID
	}
	return uid, nil
}

func parseScheduledAt(raw string) (pgtype.Timestamptz, error) {
	var ts pgtype.Timestamptz
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ts, ErrScheduledAtRequired
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var t time.Time
	var err error
	for _, layout := range layouts {
		if t, err = time.Parse(layout, raw); err == nil {
			if err := ts.Scan(t); err != nil {
				return ts, ErrInvalidScheduledAt
			}
			return ts, nil
		}
	}
	return ts, ErrInvalidScheduledAt
}

func parseMeetingDescription(description *string) pgtype.Text {
	var d pgtype.Text
	if description == nil {
		return d
	}
	trimmed := strings.TrimSpace(*description)
	if trimmed == "" {
		return d
	}
	if err := d.Scan(trimmed); err != nil {
		return pgtype.Text{}
	}
	return d
}

func (s *MeetingService) requireMeetingGroup(ctx context.Context, gid pgtype.UUID) error {
	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return err
	}
	return nil
}

func (s *MeetingService) requireMembership(ctx context.Context, gid, uid pgtype.UUID) error {
	isMember, err := s.repo.IsMember(ctx, gid, uid)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrForbidden
	}
	return nil
}

// CreateMeeting agenda una reunión: creación local síncrona + notificaciones
// in-app a los miembros (excepto el creador). Las integraciones externas se
// disparan en background y son best-effort.
func (s *MeetingService) CreateMeeting(ctx context.Context, groupID, userID, title string, description *string, scheduledAt string, notifyDiscord *bool) (sqlc.SocialMeeting, error) {
	title, err := validateTitle(title)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}
	scheduled, err := parseScheduledAt(scheduledAt)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}

	notify := true
	if notifyDiscord != nil {
		notify = *notifyDiscord
	}

	if err := s.requireMeetingGroup(ctx, gid); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return sqlc.SocialMeeting{}, err
	}

	meeting, err := s.repo.CreateMeetingWithNotifications(ctx, sqlc.CreateMeetingParams{
		GroupID:         gid,
		Title:           title,
		Description:     parseMeetingDescription(description),
		ScheduledAt:     scheduled,
		CreatedByUserID: uid,
		NotifyDiscord:   notify,
	})
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}

	GoBestEffort("meetings", func(ctx context.Context) {
		s.notifier.OnMeetingCreated(ctx, meeting)
	})

	return meeting, nil
}

// ListUpcomingMeetings devuelve las reuniones del grupo con scheduled_at >= now,
// en orden cronológico. Solo para miembros.
func (s *MeetingService) ListUpcomingMeetings(ctx context.Context, groupID, userID string, now time.Time) ([]sqlc.SocialMeeting, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return nil, err
	}
	uid, err := parseMeetingUserUUID(userID)
	if err != nil {
		return nil, err
	}
	if err := s.requireMeetingGroup(ctx, gid); err != nil {
		return nil, err
	}
	if err := s.requireMembership(ctx, gid, uid); err != nil {
		return nil, err
	}
	all, err := s.repo.ListMeetingsByGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	upcoming := make([]sqlc.SocialMeeting, 0, len(all))
	for _, m := range all {
		if m.ScheduledAt.Valid && !m.ScheduledAt.Time.Before(now) {
			upcoming = append(upcoming, m)
		}
	}
	return upcoming, nil
}
