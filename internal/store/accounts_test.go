package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/t0mer/raptor/internal/models"
)

func TestUserCRUDAndBootstrapCount(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)

	if n, _ := s.CountUsers(ctx); n != 0 {
		t.Fatalf("CountUsers = %d, want 0 (bootstrap)", n)
	}

	u := &models.User{ID: uuid.NewString(), Email: "Admin@example.com", PasswordHash: "hash", Role: models.RoleAdmin}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if n, _ := s.CountUsers(ctx); n != 1 {
		t.Errorf("CountUsers = %d, want 1", n)
	}

	// Case-insensitive email lookup.
	got, err := s.GetUserByEmail(ctx, "admin@example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetUserByEmail: %v / %v", err, got)
	}
	if !got.IsAdmin() {
		t.Error("expected admin role")
	}

	// Duplicate email rejected by unique index.
	dup := &models.User{ID: uuid.NewString(), Email: "admin@example.com", PasswordHash: "x"}
	if err := s.CreateUser(ctx, dup); err == nil {
		t.Error("expected unique-email violation")
	}

	u.Role = models.RoleUser
	if err := s.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetUser(ctx, u.ID)
	if got.Role != models.RoleUser {
		t.Errorf("role = %q after update", got.Role)
	}
}

func TestAPIKeyLifecycleAndCascade(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u := &models.User{ID: uuid.NewString(), Email: "a@b.com", PasswordHash: "h"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	k := &models.APIKey{ID: uuid.NewString(), UserID: u.ID, Name: "ci", KeyHash: "deadbeef"}
	if err := s.CreateAPIKey(ctx, k); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAPIKeyByHash(ctx, "deadbeef")
	if err != nil || got.ID != k.ID {
		t.Fatalf("GetAPIKeyByHash: %v / %v", err, got)
	}
	if err := s.TouchAPIKey(ctx, k.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAPIKeyByHash(ctx, "deadbeef")
	if got.LastUsedAt == nil {
		t.Error("last_used_at not set")
	}
	keys, _ := s.ListAPIKeys(ctx, u.ID)
	if len(keys) != 1 {
		t.Errorf("ListAPIKeys = %d", len(keys))
	}

	// Deleting the user cascades to keys.
	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPIKeyByHash(ctx, "deadbeef"); err != ErrNotFound {
		t.Errorf("api key not cascade-deleted: %v", err)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	u := &models.User{ID: uuid.NewString(), Email: "s@b.com", PasswordHash: "h"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	sess := &models.Session{ID: uuid.NewString(), UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession(ctx, sess.ID)
	if err != nil || got.UserID != u.ID {
		t.Fatalf("GetSession: %v / %v", err, got)
	}
	if err := s.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID); err != ErrNotFound {
		t.Errorf("session not deleted: %v", err)
	}

	// Expired session purge.
	old := &models.Session{ID: uuid.NewString(), UserID: u.ID, ExpiresAt: time.Now().Add(-time.Hour)}
	_ = s.CreateSession(ctx, old)
	if err := s.DeleteExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, old.ID); err != ErrNotFound {
		t.Errorf("expired session not purged: %v", err)
	}
}
