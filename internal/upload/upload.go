package upload

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID                   string
	Root, Relative, Temp string
	Total, Received      int64
	Next                 int
	Strategy             string
	Created              time.Time
}
type Manager struct {
	Storage  string
	Precheck func(root string, total int64) error
	Commit   func(root string, delta int64) error
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager(storage string) *Manager {
	return &Manager{Storage: storage, sessions: map[string]*Session{}}
}
func (m *Manager) Create(root, relative string, total int64, strategy string) (*Session, error) {
	if total < 0 {
		return nil, errors.New("invalid total size")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, errors.New("invalid upload path")
	}
	relative = filepath.ToSlash(clean)
	if strategy != "overwrite" && strategy != "skip" && strategy != "rename" {
		strategy = "overwrite"
	}
	id := uuid.NewString()
	temp := filepath.Join(m.Storage, ".ew-tmp", id)
	if err := os.MkdirAll(temp, 0750); err != nil {
		return nil, err
	}
	s := &Session{ID: id, Root: root, Relative: relative, Temp: filepath.Join(temp, "payload"), Total: total, Strategy: strategy, Created: time.Now()}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}
func (m *Manager) Chunk(id string, n int, body io.Reader) (int64, error) {
	m.mu.Lock()
	s := m.sessions[id]
	if s != nil && n != s.Next {
		s = nil
	}
	m.mu.Unlock()
	if s == nil {
		return 0, errors.New("invalid or out-of-order chunk")
	}
	f, err := os.OpenFile(s.Temp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, err
	}
	written, err := io.Copy(f, body)
	ce := f.Close()
	if err != nil {
		return written, err
	}
	if ce != nil {
		return written, ce
	}
	m.mu.Lock()
	s.Received += written
	s.Next++
	m.mu.Unlock()
	return written, nil
}
func (m *Manager) Complete(id string) (string, error) {
	m.mu.Lock()
	s := m.sessions[id]
	if s != nil {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if s == nil {
		return "", errors.New("unknown upload")
	}
	defer os.RemoveAll(filepath.Dir(s.Temp))
	if s.Received != s.Total {
		return "", errors.New("upload size mismatch")
	}
	destination := filepath.Join(s.Root, filepath.FromSlash(s.Relative))
	if err := os.MkdirAll(filepath.Dir(destination), 0750); err != nil {
		return "", err
	}
	if _, err := os.Stat(destination); err == nil {
		switch s.Strategy {
		case "skip":
			return destination, nil
		case "rename":
			destination = uniqueName(destination)
		}
	}
	if err := os.Rename(s.Temp, destination); err != nil {
		return "", err
	}
	if m.Commit != nil {
		if err := m.Commit(s.Root, s.Received); err != nil {
			return "", err
		}
	}
	return destination, nil
}
func uniqueName(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := base + " (" + strconv.Itoa(i) + ")" + ext
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
func (m *Manager) Cleanup(older time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cut := time.Now().Add(-older)
	for id, s := range m.sessions {
		if s.Created.Before(cut) {
			_ = os.RemoveAll(filepath.Dir(s.Temp))
			delete(m.sessions, id)
		}
	}
}

type Input struct {
	Path     string `json:"path"`
	Total    int64  `json:"total_size"`
	Strategy string `json:"conflict"`
}

func (m *Manager) Handler(root func(*http.Request) (string, error), readOnly func(*http.Request) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if readOnly(r) {
			http.Error(w, "read-only", 403)
			return
		}
		base, err := root(r)
		if err != nil {
			http.Error(w, "forbidden", 403)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/uploads"), "/")
		if len(parts) < 2 || parts[1] == "" {
			if r.Method != "POST" {
				http.NotFound(w, r)
				return
			}
			var in Input
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			if m.Precheck != nil {
				if err := m.Precheck(base, in.Total); err != nil {
					http.Error(w, err.Error(), 413)
					return
				}
			}
			relative := filepath.ToSlash(strings.TrimPrefix(in.Path, "/"))
			s, err := m.Create(base, relative, in.Total, in.Strategy)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			write(w, 201, map[string]string{"id": s.ID})
			return
		}
		id := parts[1]
		if len(parts) >= 4 && parts[2] == "chunks" && r.Method == http.MethodPut {
			n, _ := strconv.Atoi(parts[3])
			written, err := m.Chunk(id, n, r.Body)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			write(w, 200, map[string]int64{"written": written})
			return
		}
		if len(parts) >= 3 && parts[2] == "complete" && r.Method == http.MethodPost {
			destination, err := m.Complete(id)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			write(w, 200, map[string]string{"path": destination})
			return
		}
		http.NotFound(w, r)
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
