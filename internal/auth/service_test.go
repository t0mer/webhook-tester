package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/t0mer/raptor/internal/store"
)

func newService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return NewService(st), st
}

func TestBootstrapAndLogin(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)

	if svc.Bootstrapped(ctx) {
		t.Fatal("should not be bootstrapped initially")
	}
	seeded, err := svc.SeedAdminIfEmpty(ctx, "admin@example.com", "hunter2-strong")
	if err != nil || !seeded {
		t.Fatalf("SeedAdminIfEmpty: %v seeded=%v", err, seeded)
	}
	if !svc.Bootstrapped(ctx) {
		t.Error("should be bootstrapped after seeding")
	}
	// Seeding again is a no-op.
	seeded, _ = svc.SeedAdminIfEmpty(ctx, "other@example.com", "pw")
	if seeded {
		t.Error("second seed should be a no-op")
	}

	u, err := svc.Login(ctx, "admin@example.com", "hunter2-strong")
	if err != nil || !u.IsAdmin() {
		t.Fatalf("Login: %v / %+v", err, u)
	}
	if _, err := svc.Login(ctx, "admin@example.com", "wrong"); err != ErrInvalidCredentials {
		t.Errorf("bad password = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Login(ctx, "nobody@example.com", "x"); err != ErrInvalidCredentials {
		t.Errorf("unknown user = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	u, _ := svc.CreateUser(ctx, "s@b.com", "password123", "user")

	sess, err := svc.StartSession(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.UserBySession(ctx, sess.ID)
	if err != nil || got.ID != u.ID {
		t.Fatalf("UserBySession: %v / %v", err, got)
	}
	if err := svc.EndSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UserBySession(ctx, sess.ID); err == nil {
		t.Error("session should be invalid after logout")
	}
}

func TestAPIKeyResolve(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	u, _ := svc.CreateUser(ctx, "k@b.com", "password123", "user")

	plain, _, err := svc.IssueAPIKey(ctx, u.ID, "ci")
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.UserByAPIKey(ctx, plain)
	if err != nil || got.ID != u.ID {
		t.Fatalf("UserByAPIKey: %v / %v", err, got)
	}
	if _, err := svc.UserByAPIKey(ctx, "rpt_bogus"); err == nil {
		t.Error("bogus key should not resolve")
	}
}
