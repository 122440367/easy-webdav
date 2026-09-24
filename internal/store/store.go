package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Store struct{ DB *sql.DB }

type User struct {
	ID                                                int64
	Username, PasswordHash, Role, RootDir, Permission string
	Quota, CreatedAt                                  int64
	Disabled                                          bool
}

type Session struct {
	ID, UserID string
	LastSeen   int64
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	s := &Store{DB: db}
	if err = s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error                   { return s.DB.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.DB.PingContext(ctx) }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	for version := 1; ; version++ {
		name := fmt.Sprintf("migrations/%03d.sql", version)
		body, err := migrationFiles.ReadFile(name)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			break
		}
		var applied int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&applied); err != nil {
			return err
		}
		if applied != 0 {
			continue
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, time.Now().Unix())
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", version, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, u User) (int64, error) {
	result, err := s.DB.ExecContext(ctx, `INSERT INTO users(username,password_hash,role,root_dir,permission,quota,disabled,created_at) VALUES(?,?,?,?,?,?,?,?)`, u.Username, u.PasswordHash, u.Role, u.RootDir, u.Permission, u.Quota, u.Disabled, u.CreatedAt)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return s.scanUser(s.DB.QueryRowContext(ctx, `SELECT id,username,password_hash,role,root_dir,permission,quota,disabled,created_at FROM users WHERE id=?`, id))
}
func (s *Store) UserByName(ctx context.Context, name string) (User, error) {
	return s.scanUser(s.DB.QueryRowContext(ctx, `SELECT id,username,password_hash,role,root_dir,permission,quota,disabled,created_at FROM users WHERE username=?`, name))
}
func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role='admin' AND disabled=0`).Scan(&n)
	return n, err
}
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,username,password_hash,role,root_dir,permission,quota,disabled,created_at FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var result []User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}
func (s *Store) UpdateUser(ctx context.Context, id int64, username, rootDir, permission string, quota int64, disabled bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET username=?,root_dir=?,permission=?,quota=?,disabled=? WHERE id=?`, username, rootDir, permission, quota, disabled, id)
	return err
}
func (s *Store) UpdateUserPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

func (s *Store) CreateSession(ctx context.Context, session Session) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO sessions(id,user_id,last_seen) VALUES(?,?,?)`, session.ID, session.UserID, session.LastSeen)
	return err
}
func (s *Store) Session(ctx context.Context, id string, maxIdle time.Time) (Session, error) {
	var v Session
	err := s.DB.QueryRowContext(ctx, `SELECT id,user_id,last_seen FROM sessions WHERE id=? AND last_seen>=?`, id, maxIdle.Unix()).Scan(&v.ID, &v.UserID, &v.LastSeen)
	return v, err
}
func (s *Store) TouchSession(ctx context.Context, id string, now time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen=? WHERE id=?`, now.Unix(), id)
	return err
}
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}
func (s *Store) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID)
	return err
}
func (s *Store) DeleteUserSessionsExcept(ctx context.Context, userID, keep string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=? AND id<>?`, userID, keep)
	return err
}

func (s *Store) Usage(ctx context.Context, root string) (int64, error) {
	var n int64
	err := s.DB.QueryRowContext(ctx, `SELECT bytes FROM usage WHERE root_dir=?`, root).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}
func (s *Store) AddUsage(ctx context.Context, root string, delta int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO usage(root_dir,bytes,updated_at) VALUES(?,?,?) ON CONFLICT(root_dir) DO UPDATE SET bytes=bytes+excluded.bytes,updated_at=excluded.updated_at`, root, delta, time.Now().Unix())
	return err
}
func (s *Store) SetUsage(ctx context.Context, root string, bytes int64) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO usage(root_dir,bytes,updated_at) VALUES(?,?,?) ON CONFLICT(root_dir) DO UPDATE SET bytes=excluded.bytes,updated_at=excluded.updated_at`, root, bytes, time.Now().Unix())
	return err
}
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT key,value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanUserRow(row rowScanner) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.RootDir, &u.Permission, &u.Quota, &u.Disabled, &u.CreatedAt)
	return u, err
}
func (s *Store) scanUser(row *sql.Row) (User, error) { return scanUserRow(row) }
