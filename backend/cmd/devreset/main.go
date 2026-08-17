// Command devreset resets a user's password.
//
// DEVELOPMENT ONLY, and it refuses to run outside it. There is no admin screen
// for resetting someone else's password yet, and during a local end-to-end run
// the seeded administrator's password is whatever ADMIN_PASSWORD happened to be
// when the database was first seeded — which nobody remembers a week later.
//
//	APP_ENV=development DB_URL=... go run ./cmd/devreset user@example.tj 'NewPass!1'
//
// The APP_ENV guard is not decoration. A binary that can silently take over any
// account is exactly the thing that should not be runnable against production
// by someone who copied a command out of a README.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qoim/samari/backend/internal/auth"
)

func main() {
	if os.Getenv("APP_ENV") != "development" {
		log.Fatal("devreset: refuses to run unless APP_ENV=development")
	}
	if len(os.Args) != 3 {
		log.Fatal("usage: devreset <email> <new-password>")
	}
	email, password := os.Args[1], os.Args[2]

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("devreset: hash: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), os.Getenv("DB_URL"))
	if err != nil {
		log.Fatalf("devreset: connect: %v", err)
	}
	defer pool.Close()

	// Clears the lockout too: a forgotten password usually comes with a locked
	// account, and resetting one without the other leaves the user still locked.
	tag, err := pool.Exec(context.Background(), `
		UPDATE users
		SET password_hash = $2, failed_attempts = 0, locked_until = NULL
		WHERE email = $1 AND deleted_at IS NULL`, email, hash)
	if err != nil {
		log.Fatalf("devreset: update: %v", err)
	}
	if tag.RowsAffected() == 0 {
		log.Fatalf("devreset: no active user with email %s", email)
	}
	fmt.Printf("devreset: password reset for %s\n", email)
}
