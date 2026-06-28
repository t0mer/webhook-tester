package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/t0mer/raptor/internal/auth"
	"github.com/t0mer/raptor/internal/models"
	"github.com/t0mer/raptor/internal/store"
)

// resetPassword interactively sets an admin password: it updates the user if the
// email exists, otherwise creates a new admin. Used to recover access.
func resetPassword(st *store.Store, svc *auth.Service) error {
	ctx := context.Background()
	reader := bufio.NewReader(os.Stdin) // single reader shared across prompts

	fmt.Print("Admin email: ")
	emailLine, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read email: %w", err)
	}
	email := strings.TrimSpace(strings.ToLower(emailLine))
	if email == "" {
		return errors.New("email is required")
	}

	password, err := promptPassword(reader, "New password: ")
	if err != nil {
		return err
	}
	confirm, err := promptPassword(reader, "Confirm password: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("passwords do not match")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}

	existing, err := st.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		hash, herr := auth.HashPassword(password)
		if herr != nil {
			return herr
		}
		existing.PasswordHash = hash
		existing.Role = models.RoleAdmin
		if err := st.UpdateUser(ctx, existing); err != nil {
			return err
		}
		fmt.Printf("Password updated for %s\n", email)
	case errors.Is(err, store.ErrNotFound):
		if _, err := svc.CreateUser(ctx, email, password, models.RoleAdmin); err != nil {
			return err
		}
		fmt.Printf("Admin %s created\n", email)
	default:
		return err
	}
	return nil
}

func promptPassword(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		// Non-interactive (CI/tests): read a line from the shared reader.
		line, err := reader.ReadString('\n')
		return strings.TrimSpace(line), err
	}
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
