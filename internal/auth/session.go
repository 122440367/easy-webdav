package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/122440367/easy-webdav/internal/store"
)

const SessionCookie = "ew_session"
const SessionIdle = 7 * 24 * time.Hour

type Sessions struct{ Store *store.Store }

func (s Sessions) Create(ctx context.Context, userID int64, secure bool) (*http.Cookie, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	id := base64.RawURLEncoding.EncodeToString(buf)
	if err := s.Store.CreateSession(ctx, store.Session{ID: id, UserID: fmtInt(userID), LastSeen: time.Now().Unix()}); err != nil {
		return nil, err
	}
	return &http.Cookie{Name: SessionCookie, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure}, nil
}

func (s Sessions) Lookup(ctx context.Context, id string) (store.User, error) {
	session, err := s.Store.Session(ctx, id, time.Now().Add(-SessionIdle))
	if err != nil {
		return store.User{}, err
	}
	var user store.User
	var userID int64
	if _, err := fmtScan(session.UserID, &userID); err != nil {
		return user, err
	}
	user, err = s.Store.UserByID(ctx, userID)
	if err != nil || user.Disabled {
		return store.User{}, errors.New("invalid session")
	}
	if err == nil {
		err = s.Store.TouchSession(ctx, id, time.Now())
	}
	return user, err
}

func (s Sessions) Delete(ctx context.Context, id string) error { return s.Store.DeleteSession(ctx, id) }
func (s Sessions) RevokeUser(ctx context.Context, userID int64) error {
	return s.Store.DeleteUserSessions(ctx, fmtInt(userID))
}
func (s Sessions) RevokeUserExcept(ctx context.Context, userID int64, keep string) error {
	return s.Store.DeleteUserSessionsExcept(ctx, fmtInt(userID), keep)
}
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: SessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode}
}

// Kept local to avoid exposing storage-specific ID conversion helpers.
func fmtInt(n int64) string                            { return strconv.FormatInt(n, 10) }
func fmtScan(value string, target *int64) (int, error) { return fmt.Sscan(value, target) }
