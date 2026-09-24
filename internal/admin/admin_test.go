package admin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/store"
)

type fixture struct {
	dataDir, storageDir string
	env                 func(string) string
	stdout, stderr      bytes.Buffer
	stdin               string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{dataDir: filepath.Join(root, "data"), storageDir: filepath.Join(root, "data", "files")}
	values := map[string]string{"EW_DATA_DIR": f.dataDir, "EW_STORAGE_DIR": f.storageDir}
	f.env = func(key string) string { return values[key] }
	return f
}

func (f *fixture) run(t *testing.T, args ...string) int {
	t.Helper()
	f.stdout.Reset()
	f.stderr.Reset()
	return Run(args, f.env, strings.NewReader(f.stdin), &f.stdout, &f.stderr)
}

func (f *fixture) open(t *testing.T) *store.Store {
	t.Helper()
	if err := os.MkdirAll(f.dataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(f.dataDir, "easy-webdav.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCreateUserFromFlags(t *testing.T) {
	f := newFixture(t)
	if code := f.run(t, "create", "--username", "alice", "--password", "alicepassword"); code != 0 {
		t.Fatalf("exit code %d: %s", code, f.stderr.String())
	}
	if !strings.Contains(f.stdout.String(), "alice") {
		t.Fatalf("confirmation missing: %q", f.stdout.String())
	}
	db := f.open(t)
	user, err := db.UserByName(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "user" || user.Permission != "readwrite" || user.Quota != 0 || user.RootDir != "alice" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if !auth.CheckPassword(user.PasswordHash, "alicepassword") {
		t.Fatal("stored password does not verify")
	}
	if info, err := os.Stat(filepath.Join(f.storageDir, "alice")); err != nil || !info.IsDir() {
		t.Fatalf("root directory was not created: %v", err)
	}
}

func TestCreateHonoursRuntimeDefaultsAndAdminRole(t *testing.T) {
	f := newFixture(t)
	db := f.open(t)
	ctx := context.Background()
	if err := db.SetSetting(ctx, "default_quota", "1048576"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSetting(ctx, "default_permission", "read"); err != nil {
		t.Fatal(err)
	}
	if code := f.run(t, "create", "--username", "root-admin", "--password", "adminpassword", "--admin"); code != 0 {
		t.Fatalf("exit code %d: %s", code, f.stderr.String())
	}
	user, err := db.UserByName(ctx, "root-admin")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "admin" || user.Permission != "read" || user.Quota != 1048576 {
		t.Fatalf("runtime defaults not applied: %+v", user)
	}
}

func TestCreateRejectsDuplicateAndInvalidInput(t *testing.T) {
	f := newFixture(t)
	if code := f.run(t, "create", "--username", "alice", "--password", "alicepassword"); code != 0 {
		t.Fatalf("first create failed: %s", f.stderr.String())
	}
	if code := f.run(t, "create", "--username", "alice", "--password", "alicepassword"); code != 1 {
		t.Fatalf("duplicate create exit code %d, want 1", code)
	}
	if !strings.Contains(f.stderr.String(), "create user") {
		t.Fatalf("unexpected error message: %q", f.stderr.String())
	}
	if code := f.run(t, "create", "--username", "../escape", "--password", "alicepassword"); code != 2 {
		t.Fatalf("invalid username exit code %d, want 2", code)
	}
	if code := f.run(t, "create", "--username", "bob", "--password", "short"); code != 1 {
		t.Fatalf("weak password exit code %d, want 1", code)
	}
	if code := f.run(t, "create", "--username", "carol", "--password", "carolpassword", "--root-dir", "../outside"); code != 2 {
		t.Fatalf("escaping root dir exit code %d, want 2", code)
	}
}

func TestPasswordComesFromEnvironmentOrStdin(t *testing.T) {
	f := newFixture(t)
	env := f.env
	f.env = func(key string) string {
		if key == "EW_ADMIN_PASSWORD" {
			return "envpassword"
		}
		return env(key)
	}
	if code := f.run(t, "create", "--username", "envuser"); code != 0 {
		t.Fatalf("env password create failed: %s", f.stderr.String())
	}
	db := f.open(t)
	user, err := db.UserByName(context.Background(), "envuser")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(user.PasswordHash, "envpassword") {
		t.Fatal("environment password was not stored")
	}

	f.env = env
	f.stdin = "pipedpassword\n"
	if code := f.run(t, "create", "--username", "pipeduser"); code != 0 {
		t.Fatalf("stdin password create failed: %s", f.stderr.String())
	}
	piped, err := db.UserByName(context.Background(), "pipeduser")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(piped.PasswordHash, "pipedpassword") {
		t.Fatal("piped password was not stored")
	}
}

func TestResetPasswordRevokesSessions(t *testing.T) {
	f := newFixture(t)
	if code := f.run(t, "create", "--username", "alice", "--password", "alicepassword", "--admin"); code != 0 {
		t.Fatalf("create failed: %s", f.stderr.String())
	}
	db := f.open(t)
	ctx := context.Background()
	user, err := db.UserByName(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateSession(ctx, store.Session{ID: "session-1", UserID: "1", LastSeen: 1}); err != nil {
		t.Fatal(err)
	}
	const rotated = "newpassword1"
	if code := f.run(t, "reset-password", "--username", "alice", "--password", rotated); code != 0 {
		t.Fatalf("reset failed: %s", f.stderr.String())
	}
	updated, err := db.UserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CheckPassword(updated.PasswordHash, rotated) || auth.CheckPassword(updated.PasswordHash, "alicepassword") {
		t.Fatal("password was not rotated")
	}
	if _, err := db.Session(ctx, "session-1", time.Unix(0, 0)); err == nil {
		t.Fatal("existing session survived the password rotation")
	}
	if code := f.run(t, "reset-password", "--username", "missing", "--password", rotated); code != 1 {
		t.Fatalf("unknown user exit code %d, want 1", code)
	}
}

func TestListAndUsageErrors(t *testing.T) {
	f := newFixture(t)
	if code := f.run(t, "create", "--username", "alice", "--password", "alicepassword"); code != 0 {
		t.Fatalf("create failed: %s", f.stderr.String())
	}
	if code := f.run(t, "list"); code != 0 {
		t.Fatalf("list exit code %d: %s", code, f.stderr.String())
	}
	output := f.stdout.String()
	for _, want := range []string{"USERNAME", "alice", "readwrite", "unlimited", "enabled"} {
		if !strings.Contains(output, want) {
			t.Fatalf("list output missing %q: %s", want, output)
		}
	}
	if code := f.run(t); code != 2 {
		t.Fatalf("missing command exit code %d, want 2", code)
	}
	if code := f.run(t, "nonsense"); code != 2 {
		t.Fatalf("unknown command exit code %d, want 2", code)
	}
	if code := f.run(t, "reset-password"); code != 2 {
		t.Fatalf("reset-password without username exit code %d, want 2", code)
	}
}
