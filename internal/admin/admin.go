// Package admin implements the offline management commands. They exist so an
// operator can create accounts or recover a lost administrator password on a
// machine where the server is not running and no API session is available.
package admin

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/122440367/easy-webdav/internal/api"
	"github.com/122440367/easy-webdav/internal/auth"
	"github.com/122440367/easy-webdav/internal/config"
	"github.com/122440367/easy-webdav/internal/store"
)

const usage = `usage: easy-webdav admin <command> [flags]

Commands:
  create           create a user or administrator
  reset-password   set a new password and revoke that user's sessions
  list             list every account

Flags shared by create and reset-password:
  --username NAME  login name (required)
  --password VALUE password; otherwise EW_ADMIN_PASSWORD or an interactive prompt
  --config FILE    configuration file
  --data-dir DIR   data directory (defaults to EW_DATA_DIR or ./data)
  --storage-dir DIR storage directory (defaults to EW_STORAGE_DIR or <data-dir>/files)

Flags for create:
  --admin              grant the administrator role
  --role admin|user    explicit role
  --root-dir PATH      storage-relative root directory (defaults to the username)
  --permission VALUE   read or readwrite (defaults to the runtime setting)
  --quota BYTES        quota in bytes, 0 means unlimited (defaults to the runtime setting)
`

type runner struct {
	getenv   func(string) string
	rawStdin io.Reader
	stdin    *bufio.Reader
	stdout   io.Writer
	stderr   io.Writer
}

// Run executes one admin subcommand and returns a process exit code.
func Run(args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	if getenv == nil {
		getenv = os.Getenv
	}
	r := &runner{getenv: getenv, rawStdin: stdin, stdin: bufio.NewReader(stdin), stdout: stdout, stderr: stderr}
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "create":
		return r.create(args[1:])
	case "reset-password", "passwd", "password":
		return r.resetPassword(args[1:])
	case "list":
		return r.list(args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown admin command %q\n\n%s", args[0], usage)
		return 2
	}
}

func (r *runner) fail(err error) {
	fmt.Fprintf(r.stderr, "error: %v\n", err)
}

type configFlags struct {
	config, dataDir, storageDir string
}

func (c *configFlags) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "", "configuration file")
	fs.StringVar(&c.dataDir, "data-dir", "", "data directory")
	fs.StringVar(&c.storageDir, "storage-dir", "", "storage directory")
}

// load resolves the effective configuration, honouring the config file,
// EW_* environment variables and the flags the subcommand accepted.
func (r *runner) load(c *configFlags) (config.Config, error) {
	passthrough := []string{}
	if c.config != "" {
		passthrough = append(passthrough, "--config", c.config)
	}
	if c.dataDir != "" {
		passthrough = append(passthrough, "--data-dir", c.dataDir)
	}
	if c.storageDir != "" {
		passthrough = append(passthrough, "--storage-dir", c.storageDir)
	}
	effective, err := config.Load(passthrough, r.getenv)
	if err != nil {
		return config.Config{}, err
	}
	return effective.Config, nil
}

func openStore(cfg config.Config) (*store.Store, func(), error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, nil, err
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "easy-webdav.db"))
	if err != nil {
		return nil, nil, err
	}
	return db, func() { _ = db.Close() }, nil
}

// password prefers an explicit flag, then the environment, then a prompt.
func (r *runner) password(value string) (string, error) {
	if value != "" {
		return value, nil
	}
	if fromEnv := r.getenv("EW_ADMIN_PASSWORD"); fromEnv != "" {
		return fromEnv, nil
	}
	return readSecret(r.rawStdin, r.stdin, r.stdout, "Password: ")
}

func (r *runner) create(args []string) int {
	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	fs.SetOutput(r.stderr)
	cfgFlags := &configFlags{}
	cfgFlags.register(fs)
	username := fs.String("username", "", "login name")
	password := fs.String("password", "", "password")
	rootDir := fs.String("root-dir", "", "storage-relative root directory")
	permission := fs.String("permission", "", "read or readwrite")
	quota := fs.Int64("quota", -1, "quota in bytes, 0 means unlimited")
	asAdmin := fs.Bool("admin", false, "grant the administrator role")
	role := fs.String("role", "", "admin or user")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *role != "" && *role != "admin" && *role != "user" {
		r.fail(errors.New("--role must be admin or user"))
		return 2
	}
	if err := api.ValidateUsername(*username); err != nil {
		r.fail(fmt.Errorf("--username: %w", err))
		return 2
	}
	if *permission != "" && *permission != "read" && *permission != "readwrite" {
		r.fail(errors.New("--permission must be read or readwrite"))
		return 2
	}
	cfg, err := r.load(cfgFlags)
	if err != nil {
		r.fail(err)
		return 2
	}
	db, closeStore, err := openStore(cfg)
	if err != nil {
		r.fail(err)
		return 1
	}
	defer closeStore()
	ctx := context.Background()
	settings, err := db.Settings(ctx)
	if err != nil {
		r.fail(err)
		return 1
	}
	resolvedPermission := *permission
	if resolvedPermission == "" {
		resolvedPermission = settings["default_permission"]
	}
	if resolvedPermission != "read" && resolvedPermission != "readwrite" {
		resolvedPermission = "readwrite"
	}
	resolvedQuota := *quota
	if resolvedQuota < 0 {
		resolvedQuota = 0
		if configured := settings["default_quota"]; configured != "" {
			resolvedQuota, _ = strconv.ParseInt(configured, 10, 64)
		}
	}
	root := *username
	if *rootDir != "" {
		root, err = api.ValidateRootDir(*rootDir)
		if err != nil {
			r.fail(fmt.Errorf("--root-dir: %w", err))
			return 2
		}
	}
	secret, err := r.password(*password)
	if err != nil {
		r.fail(err)
		return 1
	}
	hash, err := auth.HashPassword(secret)
	if err != nil {
		r.fail(err)
		return 1
	}
	if err := os.MkdirAll(filepath.Join(cfg.StorageDir, root), 0o750); err != nil {
		r.fail(fmt.Errorf("create root directory: %w", err))
		return 1
	}
	resolvedRole := "user"
	if *asAdmin || *role == "admin" {
		resolvedRole = "admin"
	}
	id, err := db.CreateUser(ctx, store.User{Username: *username, PasswordHash: hash, Role: resolvedRole, RootDir: root, Permission: resolvedPermission, Quota: resolvedQuota})
	if err != nil {
		r.fail(fmt.Errorf("create user: %w", err))
		return 1
	}
	fmt.Fprintf(r.stdout, "created %s user %s (id %d, %s, quota %s, root %s)\n",
		resolvedRole, *username, id, resolvedPermission, describeQuota(resolvedQuota), root)
	return 0
}

func (r *runner) resetPassword(args []string) int {
	fs := flag.NewFlagSet("admin reset-password", flag.ContinueOnError)
	fs.SetOutput(r.stderr)
	cfgFlags := &configFlags{}
	cfgFlags.register(fs)
	username := fs.String("username", "", "login name")
	password := fs.String("password", "", "password")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *username == "" {
		r.fail(errors.New("--username is required"))
		return 2
	}
	cfg, err := r.load(cfgFlags)
	if err != nil {
		r.fail(err)
		return 2
	}
	db, closeStore, err := openStore(cfg)
	if err != nil {
		r.fail(err)
		return 1
	}
	defer closeStore()
	ctx := context.Background()
	user, err := db.UserByName(ctx, *username)
	if err != nil {
		r.fail(fmt.Errorf("user %q not found", *username))
		return 1
	}
	secret, err := r.password(*password)
	if err != nil {
		r.fail(err)
		return 1
	}
	hash, err := auth.HashPassword(secret)
	if err != nil {
		r.fail(err)
		return 1
	}
	if err := db.UpdateUserPassword(ctx, user.ID, hash); err != nil {
		r.fail(err)
		return 1
	}
	// WebDAV and panel sessions must not survive a password change.
	if err := db.DeleteUserSessions(ctx, strconv.FormatInt(user.ID, 10)); err != nil {
		r.fail(err)
		return 1
	}
	fmt.Fprintf(r.stdout, "password updated for %s; existing sessions revoked\n", user.Username)
	return 0
}

func (r *runner) list(args []string) int {
	fs := flag.NewFlagSet("admin list", flag.ContinueOnError)
	fs.SetOutput(r.stderr)
	cfgFlags := &configFlags{}
	cfgFlags.register(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := r.load(cfgFlags)
	if err != nil {
		r.fail(err)
		return 2
	}
	db, closeStore, err := openStore(cfg)
	if err != nil {
		r.fail(err)
		return 1
	}
	defer closeStore()
	users, err := db.ListUsers(context.Background())
	if err != nil {
		r.fail(err)
		return 1
	}
	fmt.Fprintf(r.stdout, "%-24s %-6s %-10s %14s %-9s %s\n", "USERNAME", "ROLE", "PERMISSION", "QUOTA", "STATUS", "ROOT")
	for _, user := range users {
		status := "enabled"
		if user.Disabled {
			status = "disabled"
		}
		fmt.Fprintf(r.stdout, "%-24s %-6s %-10s %14s %-9s %s\n", user.Username, user.Role, user.Permission, describeQuota(user.Quota), status, user.RootDir)
	}
	if len(users) == 0 {
		fmt.Fprintln(r.stdout, "no accounts yet")
	}
	return 0
}

func describeQuota(quota int64) string {
	if quota <= 0 {
		return "unlimited"
	}
	return strconv.FormatInt(quota, 10)
}

func readSecret(raw io.Reader, in *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	restore := func() {}
	if file, ok := raw.(*os.File); ok {
		if stop, err := hideInput(file); err == nil {
			restore = stop
		}
	}
	line, err := in.ReadString('\n')
	restore()
	fmt.Fprintln(out)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimRight(line, "\r\n")
	if value == "" {
		return "", errors.New("password must not be empty")
	}
	return value, nil
}
