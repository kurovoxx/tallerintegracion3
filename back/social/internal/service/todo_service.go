package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

var (
	ErrGroupNotFound  = errors.New("group not found")
	ErrTodoNotFound   = errors.New("todo not found")
	ErrBoardNotFound  = errors.New("board not found in group")
	ErrInvalidGroupID = errors.New("invalid group ID")
	ErrInvalidTodoID  = errors.New("invalid todo ID")
	ErrInvalidBoardID = errors.New("invalid board ID")
	ErrInvalidUserID  = errors.New("invalid user ID")
	ErrInvalidTitle   = errors.New("title cannot be empty")
	ErrInvalidStatus  = errors.New("invalid status: must be todo, in_progress or done")
	ErrInvalidDueDate = errors.New("invalid due_date: must be YYYY-MM-DD")
	ErrTitleTooLong   = errors.New("title too long: max 300 characters")
	ErrNothingToPatch = errors.New("nothing to update: provide at least one of title, status, assigned_to, due_date")
)

var validStatuses = map[string]struct{}{
	"todo":        {},
	"in_progress": {},
	"done":        {},
}

const defaultBoardName = "General"

type TodoService struct {
	repo *repository.TodoRepository
}

func NewTodoService(repo *repository.TodoRepository) *TodoService {
	return &TodoService{
		repo: repo,
	}
}

type TodoTaskView struct {
	Task  sqlc.SocialTodoTask
	Board sqlc.SocialTodoBoard
}

func parseGroupUUID(groupID string) (pgtype.UUID, error) {
	var gid pgtype.UUID
	if _, err := uuid.Parse(strings.TrimSpace(groupID)); err != nil {
		return gid, ErrInvalidGroupID
	}
	if err := gid.Scan(strings.TrimSpace(groupID)); err != nil {
		return gid, ErrInvalidGroupID
	}
	return gid, nil
}

func (s *TodoService) requireGroup(ctx context.Context, gid pgtype.UUID) error {
	if _, err := s.repo.GetGroupByID(ctx, gid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrGroupNotFound
		}
		return err
	}
	return nil
}

func (s *TodoService) resolveBoard(ctx context.Context, gid pgtype.UUID, boardID string) (sqlc.SocialTodoBoard, error) {
	boardID = strings.TrimSpace(boardID)
	if boardID != "" {
		var bid pgtype.UUID
		if _, err := uuid.Parse(boardID); err != nil {
			return sqlc.SocialTodoBoard{}, ErrInvalidBoardID
		}
		if err := bid.Scan(boardID); err != nil {
			return sqlc.SocialTodoBoard{}, ErrInvalidBoardID
		}
		board, err := s.repo.GetBoardByID(ctx, bid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return sqlc.SocialTodoBoard{}, ErrBoardNotFound
			}
			return sqlc.SocialTodoBoard{}, err
		}
		if board.GroupID != gid {
			return sqlc.SocialTodoBoard{}, ErrBoardNotFound
		}
		return board, nil
	}

	boards, err := s.repo.ListBoardsByGroup(ctx, gid)
	if err != nil {
		return sqlc.SocialTodoBoard{}, err
	}
	if len(boards) > 0 {
		return boards[0], nil
	}
	return s.repo.CreateBoard(ctx, sqlc.CreateBoardParams{GroupID: gid, Name: defaultBoardName})
}

func parseAssignee(assignedTo string) (pgtype.UUID, error) {
	var auid pgtype.UUID
	assignedTo = strings.TrimSpace(assignedTo)
	if assignedTo == "" {
		return auid, nil
	}
	if _, err := uuid.Parse(assignedTo); err != nil {
		return auid, ErrInvalidUserID
	}
	if err := auid.Scan(assignedTo); err != nil {
		return auid, ErrInvalidUserID
	}
	return auid, nil
}

func parseDueDate(dueDate string) (pgtype.Date, error) {
	var d pgtype.Date
	dueDate = strings.TrimSpace(dueDate)
	if dueDate == "" {
		return d, nil
	}
	if _, err := time.Parse("2006-01-02", dueDate); err != nil {
		return d, ErrInvalidDueDate
	}
	if err := d.Scan(dueDate); err != nil {
		return d, ErrInvalidDueDate
	}
	return d, nil
}

func validateTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrInvalidTitle
	}
	if len([]rune(title)) > 300 {
		return "", ErrTitleTooLong
	}
	return title, nil
}

func normalizeStatus(status string) (string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "todo"
	}
	if _, ok := validStatuses[status]; !ok {
		return "", ErrInvalidStatus
	}
	return status, nil
}

func (s *TodoService) CreateTodo(ctx context.Context, groupID, boardID, title, status, assignedTo, dueDate string) (TodoTaskView, error) {
	title, err := validateTitle(title)
	if err != nil {
		return TodoTaskView{}, err
	}
	status, err = normalizeStatus(status)
	if err != nil {
		return TodoTaskView{}, err
	}
	assignee, err := parseAssignee(assignedTo)
	if err != nil {
		return TodoTaskView{}, err
	}
	due, err := parseDueDate(dueDate)
	if err != nil {
		return TodoTaskView{}, err
	}

	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return TodoTaskView{}, err
	}
	if err := s.requireGroup(ctx, gid); err != nil {
		return TodoTaskView{}, err
	}
	board, err := s.resolveBoard(ctx, gid, boardID)
	if err != nil {
		return TodoTaskView{}, err
	}

	task, err := s.repo.CreateTask(ctx, sqlc.CreateTodoTaskParams{
		BoardID:        board.ID,
		Title:          title,
		Status:         status,
		AssigneeUserID: assignee,
		DueDate:        due,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return TodoTaskView{}, ErrBoardNotFound
		}
		return TodoTaskView{}, err
	}
	return TodoTaskView{Task: task, Board: board}, nil
}

func (s *TodoService) ListTodos(ctx context.Context, groupID, status, boardID string) ([]TodoTaskView, error) {
	gid, err := parseGroupUUID(groupID)
	if err != nil {
		return nil, err
	}

	status = strings.TrimSpace(status)
	if status != "" {
		if _, ok := validStatuses[status]; !ok {
			return nil, ErrInvalidStatus
		}
	}

	if err := s.requireGroup(ctx, gid); err != nil {
		return nil, err
	}

	var bid pgtype.UUID
	if strings.TrimSpace(boardID) != "" {
		board, err := s.resolveBoard(ctx, gid, boardID)
		if err != nil {
			return nil, err
		}
		bid = board.ID
	}

	rows, err := s.repo.ListTasksByGroup(ctx, sqlc.ListTodoTasksByGroupParams{
		GroupID: gid,
		Status:  status,
		BoardID: bid,
	})
	if err != nil {
		return nil, err
	}

	views := make([]TodoTaskView, 0, len(rows))
	for _, r := range rows {
		views = append(views, TodoTaskView{
			Task: sqlc.SocialTodoTask{
				ID:             r.ID,
				BoardID:        r.BoardID,
				Title:          r.Title,
				Status:         r.Status,
				AssigneeUserID: r.AssigneeUserID,
				DueDate:        r.DueDate,
				CreatedAt:      r.CreatedAt,
				UpdatedAt:      r.UpdatedAt,
			},
			Board: sqlc.SocialTodoBoard{
				ID:      r.BoardID,
				GroupID: gid,
				Name:    r.BoardName,
			},
		})
	}
	return views, nil
}

func (s *TodoService) UpdateTodo(ctx context.Context, todoID string, title, status, assignedTo, dueDate *string) (TodoTaskView, error) {
	if title == nil && status == nil && assignedTo == nil && dueDate == nil {
		return TodoTaskView{}, ErrNothingToPatch
	}

	todoID = strings.TrimSpace(todoID)
	if _, err := uuid.Parse(todoID); err != nil {
		return TodoTaskView{}, ErrInvalidTodoID
	}
	var tid pgtype.UUID
	if err := tid.Scan(todoID); err != nil {
		return TodoTaskView{}, ErrInvalidTodoID
	}

	current, err := s.repo.GetTaskByID(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TodoTaskView{}, ErrTodoNotFound
		}
		return TodoTaskView{}, err
	}

	arg := sqlc.UpdateTodoTaskParams{
		ID:             current.ID,
		Title:          current.Title,
		Status:         current.Status,
		AssigneeUserID: current.AssigneeUserID,
		DueDate:        current.DueDate,
	}

	if title != nil {
		t, err := validateTitle(*title)
		if err != nil {
			return TodoTaskView{}, err
		}
		arg.Title = t
	}
	if status != nil {
		if strings.TrimSpace(*status) == "" {
			return TodoTaskView{}, ErrInvalidStatus
		}
		st, err := normalizeStatus(*status)
		if err != nil {
			return TodoTaskView{}, err
		}
		arg.Status = st
	}
	if assignedTo != nil {
		a, err := parseAssignee(*assignedTo)
		if err != nil {
			return TodoTaskView{}, err
		}
		arg.AssigneeUserID = a
	}
	if dueDate != nil {
		d, err := parseDueDate(*dueDate)
		if err != nil {
			return TodoTaskView{}, err
		}
		arg.DueDate = d
	}

	updated, err := s.repo.UpdateTask(ctx, arg)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TodoTaskView{}, ErrTodoNotFound
		}
		return TodoTaskView{}, err
	}

	board, err := s.repo.GetBoardByID(ctx, updated.BoardID)
	if err != nil {
		return TodoTaskView{}, err
	}
	return TodoTaskView{Task: updated, Board: board}, nil
}

func (s *TodoService) DeleteTodo(ctx context.Context, todoID string) (TodoTaskView, error) {
	todoID = strings.TrimSpace(todoID)
	if _, err := uuid.Parse(todoID); err != nil {
		return TodoTaskView{}, ErrInvalidTodoID
	}
	var tid pgtype.UUID
	if err := tid.Scan(todoID); err != nil {
		return TodoTaskView{}, ErrInvalidTodoID
	}

	deleted, err := s.repo.DeleteTask(ctx, tid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TodoTaskView{}, ErrTodoNotFound
		}
		return TodoTaskView{}, err
	}

	board, err := s.repo.GetBoardByID(ctx, deleted.BoardID)
	if err != nil {
		return TodoTaskView{Task: deleted}, err
	}
	return TodoTaskView{Task: deleted, Board: board}, nil
}
