package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrecedence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("listen: ':9000'\ndata_dir: "+filepath.Join(dir, "file")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "EW_CONFIG" {
			return file
		}
		if key == "EW_LISTEN" {
			return ":9100"
		}
		return ""
	}
	e, err := Load([]string{"--listen", ":9200"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if e.Config.Listen != ":9200" || e.Sources["listen"] != "flag" {
		t.Fatalf("unexpected precedence: %+v", e)
	}
}

func TestUnknownYAMLField(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(nil, func(key string) string {
		if key == "EW_CONFIG" {
			return file
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestPrintMasksPassword(t *testing.T) {
	e := Effective{Config: Config{AdminPassword: "secret-password"}, Sources: map[string]string{}}
	b, err := PrintJSON(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret-password") || !strings.Contains(string(b), "********") {
		t.Fatal("password was not masked")
	}
}
