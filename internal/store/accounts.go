package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/t0mer/raptor/internal/models"
)

// --- Users ---

// CreateUser inserts a new user.
func (s *Store) CreateUser(ctx context.Context, u *models.User) error {
	now := time.Now().UTC()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	u.UpdatedAt = now
	if u.Role == "" {
		u.Role = models.RoleUser
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, role, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		u.ID, u.Email, u.PasswordHash, u.Role, nowRFC3339(u.CreatedAt), nowRFC3339(u.UpdatedAt))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetUser returns a user by id.
func (s *Store) GetUser(ctx context.Context, id string) (*models.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE id = ?`, id))
}

// GetUserByEmail returns a user by (case-insensitive) email.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return scanUser(s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE email = ? COLLATE NOCASE`, email))
}

// ListUsers returns all users, oldest first.
func (s *Store) ListUsers(ctx context.Context) ([]*models.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, password_hash, role, created_at, updated_at FROM users ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()
	var out []*models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers returns the number of users (drives bootstrap mode).
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// UpdateUser persists email, password hash and role.
func (s *Store) UpdateUser(ctx context.Context, u *models.User) error {
	u.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=?, password_hash=?, role=?, updated_at=? WHERE id=?`,
		u.Email, u.PasswordHash, u.Role, nowRFC3339(u.UpdatedAt), u.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return requireAffected(res)
}

// DeleteUser removes a user (cascading to their keys and sessions).
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return requireAffected(res)
}

func scanUser(sc scanner) (*models.User, error) {
	var (
		u                models.User
		created, updated string
	)
	if err := sc.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &created, &updated); err != nil {
		return nil, mapNoRows(err)
	}
	u.CreatedAt, _ = parseTime(created)
	u.UpdatedAt, _ = parseTime(updated)
	return &u, nil
}

// --- API keys ---

// CreateAPIKey stores an API key (hash only).
func (s *Store) CreateAPIKey(ctx context.Context, k *models.APIKey) error {
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, name, key_hash, last_used_at, created_at) VALUES (?,?,?,?,?,?)`,
		k.ID, k.UserID, k.Name, k.KeyHash, nullTime(k.LastUsedAt), nowRFC3339(k.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert api key: %w", err)
	}
	return nil
}

// GetAPIKeyByHash looks up an API key by its SHA-256 hash.
func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (*models.APIKey, error) {
	return scanAPIKey(s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, key_hash, last_used_at, created_at FROM api_keys WHERE key_hash = ?`, hash))
}

// GetAPIKey looks up an API key by id.
func (s *Store) GetAPIKey(ctx context.Context, id string) (*models.APIKey, error) {
	return scanAPIKey(s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, key_hash, last_used_at, created_at FROM api_keys WHERE id = ?`, id))
}

// ListAPIKeys returns a user's API keys, newest first.
func (s *Store) ListAPIKeys(ctx context.Context, userID string) ([]*models.APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, key_hash, last_used_at, created_at FROM api_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()
	var out []*models.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// DeleteAPIKey removes an API key.
func (s *Store) DeleteAPIKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	return requireAffected(res)
}

// TouchAPIKey records the last-used time of an API key (best-effort).
func (s *Store) TouchAPIKey(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`, nowRFC3339(at), id)
	return err
}

func scanAPIKey(sc scanner) (*models.APIKey, error) {
	var (
		k       models.APIKey
		lastNS  sql.NullString
		created string
	)
	if err := sc.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &lastNS, &created); err != nil {
		return nil, mapNoRows(err)
	}
	k.CreatedAt, _ = parseTime(created)
	if lastNS.Valid && lastNS.String != "" {
		if t, err := parseTime(lastNS.String); err == nil {
			k.LastUsedAt = &t
		}
	}
	return &k, nil
}

// --- Sessions ---

// CreateSession stores a login session.
func (s *Store) CreateSession(ctx context.Context, sess *models.Session) error {
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at, created_at) VALUES (?,?,?,?)`,
		sess.ID, sess.UserID, nowRFC3339(sess.ExpiresAt), nowRFC3339(sess.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	return nil
}

// GetSession returns a session by id (caller checks expiry).
func (s *Store) GetSession(ctx context.Context, id string) (*models.Session, error) {
	var (
		sess             models.Session
		expires, created string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at, created_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &expires, &created)
	if err != nil {
		return nil, mapNoRows(err)
	}
	sess.ExpiresAt, _ = parseTime(expires)
	sess.CreatedAt, _ = parseTime(created)
	return &sess, nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteExpiredSessions purges sessions past their expiry.
func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, nowRFC3339(time.Now().UTC()))
	return err
}
