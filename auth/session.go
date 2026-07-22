package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"
)

const (
	SessionCookieName = "_mold_session"
	SessionDuration   = 24 * time.Hour
)

type Session struct {
	ID        string    `json:"id"`
	UserID    any       `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionManager struct {
	db *sql.DB
}

func NewSessionManager(db *sql.DB) (*SessionManager, error) {
	sm := &SessionManager{db: db}
	if err := sm.EnsureTable(context.Background()); err != nil {
		return nil, err
	}
	return sm, nil
}

func (sm *SessionManager) EnsureTable(ctx context.Context) error {
	createSQL := `CREATE TABLE IF NOT EXISTS "_mold_sessions" (
		"id" TEXT PRIMARY KEY,
		"user_id" TEXT NOT NULL,
		"username" TEXT NOT NULL,
		"role" TEXT NOT NULL,
		"expires_at" TEXT NOT NULL
	);`
	_, err := sm.db.ExecContext(ctx, createSQL)
	if err != nil {
		return fmt.Errorf("failed to create session table: %w", err)
	}
	return nil
}

func (sm *SessionManager) CreateSession(ctx context.Context, userID any, username, role string) (*Session, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}
	sessionID := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().UTC().Add(SessionDuration)

	sess := &Session{
		ID:        sessionID,
		UserID:    userID,
		Username:  username,
		Role:      role,
		ExpiresAt: expiresAt,
	}

	insertSQL := `INSERT INTO "_mold_sessions" ("id", "user_id", "username", "role", "expires_at") VALUES (?, ?, ?, ?, ?);`
	userIDStr := fmt.Sprintf("%v", userID)
	_, err := sm.db.ExecContext(ctx, insertSQL, sessionID, userIDStr, username, role, expiresAt.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	return sess, nil
}

func (sm *SessionManager) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	if sessionID == "" {
		return nil, nil
	}

	querySQL := `SELECT "id", "user_id", "username", "role", "expires_at" FROM "_mold_sessions" WHERE "id" = ?;`
	var sess Session
	var userIDStr string
	var expiresAtStr string

	err := sm.db.QueryRowContext(ctx, querySQL, sessionID).Scan(&sess.ID, &userIDStr, &sess.Username, &sess.Role, &expiresAtStr)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	parsedExp, err := time.Parse(time.RFC3339, expiresAtStr)
	if err == nil {
		sess.ExpiresAt = parsedExp
		if time.Now().UTC().After(parsedExp) {
			_ = sm.DeleteSession(ctx, sessionID)
			return nil, nil
		}
	}

	sess.UserID = userIDStr
	return &sess, nil
}

func (sm *SessionManager) DeleteSession(ctx context.Context, sessionID string) error {
	deleteSQL := `DELETE FROM "_mold_sessions" WHERE "id" = ?;`
	_, err := sm.db.ExecContext(ctx, deleteSQL, sessionID)
	return err
}

func SetSessionCookie(w http.ResponseWriter, sessionID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  time.Now().UTC().Add(SessionDuration),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}
