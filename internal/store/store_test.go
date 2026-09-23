package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestMigrationsAndConstraints(t *testing.T) {
	s, err := Open("file:test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := User{Username: "admin", PasswordHash: "hash", Role: "admin", RootDir: "admin", Permission: "readwrite", CreatedAt: time.Now().Unix()}
	if _, err = s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateUser(context.Background(), u); err == nil {
		t.Fatal("duplicate username accepted")
	}
	if err = s.migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UserByName(context.Background(), "admin"); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredSession(t *testing.T) {
	s, err := Open("file:sessions?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CreateSession(context.Background(), Session{ID: "id", UserID: "1", LastSeen: time.Now().Add(-8 * 24 * time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Session(context.Background(), "id", time.Now().Add(-7*24*time.Hour)); err != sql.ErrNoRows {
		t.Fatalf("expected expired session, got %v", err)
	}
}
