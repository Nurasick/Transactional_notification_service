package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type NotificationRecord struct {
	MessageID string
	UserID    string
	EventType string
	Status    string
	Error     string
	SentAt    time.Time
}

type ListFilter struct {
	UserID    string
	Status    string
	EventType string
	Limit     int
	Offset    int
}

type NotificationLog struct {
	db *sql.DB
}

func New(dsn string) (*NotificationLog, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &NotificationLog{db: db}, nil
}

func (l *NotificationLog) Insert(ctx context.Context, r NotificationRecord) error {
	const q = `
		INSERT INTO notifications (message_id, user_id, event_type, status, error, sent_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
		ON CONFLICT (message_id) DO NOTHING`

	_, err := l.db.ExecContext(ctx, q,
		r.MessageID, r.UserID, r.EventType, r.Status, r.Error, r.SentAt)
	return err
}

func (l *NotificationLog) List(ctx context.Context, f ListFilter) ([]NotificationRecord, error) {
	clauses := []string{}
	args := []any{}
	n := 1

	if f.UserID != "" {
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", n))
		args = append(args, f.UserID)
		n++
	}
	if f.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", n))
		args = append(args, f.Status)
		n++
	}
	if f.EventType != "" {
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", n))
		args = append(args, f.EventType)
		n++
	}

	where := "TRUE"
	if len(clauses) > 0 {
		where = strings.Join(clauses, " AND ")
	}

	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	q := fmt.Sprintf(`
		SELECT message_id, user_id, event_type, status, COALESCE(error, ''), sent_at
		FROM notifications
		WHERE %s
		ORDER BY sent_at DESC
		LIMIT %d OFFSET $%d`, where, limit, n)
	args = append(args, f.Offset)

	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []NotificationRecord
	for rows.Next() {
		var r NotificationRecord
		if err := rows.Scan(&r.MessageID, &r.UserID, &r.EventType, &r.Status, &r.Error, &r.SentAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
