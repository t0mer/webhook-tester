package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

// DefaultSessionTTL is how long a login session lasts.
const DefaultSessionTTL = 7 * 24 * time.Hour

// ErrInvalidCredentials is returned on a failed login.
var ErrInvalidCredentials = errors.New("invalid email or password")

// Service implements account authentication on top of the store.
type Service struct {
	store      *store.Store
	sessionTTL time.Duration
	now        func() time.Time
}

// NewService builds an auth Service.
func NewService(st *store.Store) *Service {
	return &Service{store: st, sessionTTL: DefaultSessionTTL, now: time.Now}
}

// Bootstrapped reports whether any user exists. Until the first user is created,
// the API stays open so an initial admin can be provisioned.
func (s *Service) Bootstrapped(ctx context.Context) bool {
	n, err := s.store.CountUsers(ctx)
	return err == nil && n > 0
}

// CreateUser creates a user with a hashed password.
func (s *Service) CreateUser(ctx context.Context, email, password, role string) (*models.User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, errors.New("email is required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = models.RoleUser
	}
	u := &models.User{ID: uuid.NewString(), Email: email, PasswordHash: hash, Role: role}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// SeedAdminIfEmpty creates an admin from env/CLI only when no users exist.
func (s *Service) SeedAdminIfEmpty(ctx context.Context, email, password string) (bool, error) {
	if email == "" || password == "" || s.Bootstrapped(ctx) {
		return false, nil
	}
	if _, err := s.CreateUser(ctx, email, password, models.RoleAdmin); err != nil {
		return false, err
	}
	return true, nil
}

// Login verifies credentials and returns the user. Password verification runs
// even for unknown emails to avoid user-enumeration timing differences.
func (s *Service) Login(ctx context.Context, email, password string) (*models.User, error) {
	u, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, store.ErrNotFound) {
		// Burn a comparison so timing doesn't reveal whether the email exists.
		_ = CheckPassword("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinv", password)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if !CheckPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// StartSession creates a session for a user and returns it.
func (s *Service) StartSession(ctx context.Context, userID string) (*models.Session, error) {
	id, err := GenerateToken(32)
	if err != nil {
		return nil, err
	}
	sess := &models.Session{ID: id, UserID: userID, ExpiresAt: s.now().UTC().Add(s.sessionTTL)}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// EndSession deletes a session (logout).
func (s *Service) EndSession(ctx context.Context, id string) error {
	return s.store.DeleteSession(ctx, id)
}

// UserBySession resolves a session id to its user, enforcing expiry.
func (s *Service) UserBySession(ctx context.Context, id string) (*models.User, error) {
	sess, err := s.store.GetSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.now().UTC().After(sess.ExpiresAt) {
		_ = s.store.DeleteSession(ctx, id)
		return nil, store.ErrNotFound
	}
	return s.store.GetUser(ctx, sess.UserID)
}

// UserByAPIKey resolves an API key to its user and records last-used.
func (s *Service) UserByAPIKey(ctx context.Context, key string) (*models.User, error) {
	k, err := s.store.GetAPIKeyByHash(ctx, HashAPIKey(key))
	if err != nil {
		return nil, err
	}
	_ = s.store.TouchAPIKey(ctx, k.ID, s.now().UTC())
	return s.store.GetUser(ctx, k.UserID)
}

// IssueAPIKey creates an API key for a user, returning the plaintext (shown once).
func (s *Service) IssueAPIKey(ctx context.Context, userID, name string) (string, *models.APIKey, error) {
	plain, hash, err := GenerateAPIKey()
	if err != nil {
		return "", nil, err
	}
	k := &models.APIKey{ID: uuid.NewString(), UserID: userID, Name: name, KeyHash: hash}
	if err := s.store.CreateAPIKey(ctx, k); err != nil {
		return "", nil, err
	}
	return plain, k, nil
}
