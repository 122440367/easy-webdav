package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen         string   `yaml:"listen" json:"listen"`
	DataDir        string   `yaml:"data_dir" json:"data_dir"`
	StorageDir     string   `yaml:"storage_dir" json:"storage_dir"`
	BaseURL        string   `yaml:"base_url" json:"base_url"`
	TLSCert        string   `yaml:"tls_cert" json:"tls_cert"`
	TLSKey         string   `yaml:"tls_key" json:"tls_key"`
	TrustedProxies []string `yaml:"trusted_proxies" json:"trusted_proxies"`
	LogFormat      string   `yaml:"log_format" json:"log_format"`
	LogLevel       string   `yaml:"log_level" json:"log_level"`
	AccessLog      string   `yaml:"access_log" json:"access_log"`
	AdminUser      string   `yaml:"admin_user" json:"admin_user"`
	AdminPassword  string   `yaml:"admin_password" json:"admin_password"`
	ConfigFile     string   `yaml:"-" json:"config_file"`
}

type Effective struct {
	Config  Config            `json:"config"`
	Sources map[string]string `json:"sources"`
}

func Defaults() Config {
	return Config{Listen: "0.0.0.0:8080", DataDir: "./data", StorageDir: "./data/files", BaseURL: "/", LogFormat: "text", LogLevel: "info", AccessLog: "errors-and-writes"}
}

func Load(args []string, getenv func(string) string) (Effective, error) {
	c := Defaults()
	sources := map[string]string{}
	for name := range fields(c) {
		sources[name] = "default"
	}

	flags := flag.NewFlagSet("easy-webdav", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "", "configuration file")
	printConfig := flags.Bool("print-config", false, "print effective configuration")
	flagValues := map[string]*string{}
	for _, name := range []string{"listen", "data-dir", "storage-dir", "base-url", "tls-cert", "tls-key", "log-format", "log-level", "access-log", "admin-user", "admin-password"} {
		flagValues[name] = flags.String(name, "", "")
	}
	if err := flags.Parse(args); err != nil {
		return Effective{}, err
	}

	envConfig := getenv("EW_CONFIG")
	explicit := *configPath
	if explicit == "" {
		explicit = envConfig
	}
	if explicit == "" {
		for _, candidate := range []string{"./config.yaml", filepath.Join(c.DataDir, "config.yaml")} {
			if _, err := os.Stat(candidate); err == nil {
				explicit = candidate
				break
			}
		}
	}
	if explicit != "" {
		if err := mergeFile(&c, explicit); err != nil {
			return Effective{}, err
		}
		c.ConfigFile, sources["config_file"] = explicit, "file"
	}
	applyEnv(&c, getenv, sources)
	if *configPath != "" {
		c.ConfigFile, sources["config_file"] = *configPath, "flag"
	}
	for name, value := range flagValues {
		if *value != "" {
			set(&c, name, *value)
			sources[fieldName(name)] = "flag"
		}
	}
	if err := Validate(c); err != nil {
		return Effective{}, err
	}
	_ = printConfig
	return Effective{Config: c, Sources: sources}, nil
}

func PrintJSON(e Effective) ([]byte, error) {
	copy := e.Config
	if copy.AdminPassword != "" {
		copy.AdminPassword = "********"
	}
	return json.MarshalIndent(Effective{Config: copy, Sources: e.Sources}, "", "  ")
}

func EnsureDirs(c Config) error {
	for _, dir := range []string{c.DataDir, c.StorageDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
		f, err := os.OpenFile(filepath.Join(dir, ".write-test"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("data directory %q is not writable: %w", dir, err)
		}
		name := f.Name()
		_ = f.Close()
		if err := os.Remove(name); err != nil {
			return fmt.Errorf("data directory %q cleanup: %w", dir, err)
		}
	}
	return nil
}

func mergeFile(c *Config, filename string) error {
	b, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read config %q: %w", filename, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		return fmt.Errorf("parse config %q: %w", filename, err)
	}
	return nil
}

func applyEnv(c *Config, getenv func(string) string, sources map[string]string) {
	values := map[string]string{"listen": "EW_LISTEN", "data-dir": "EW_DATA_DIR", "storage-dir": "EW_STORAGE_DIR", "base-url": "EW_BASE_URL", "tls-cert": "EW_TLS_CERT", "tls-key": "EW_TLS_KEY", "log-format": "EW_LOG_FORMAT", "log-level": "EW_LOG_LEVEL", "access-log": "EW_ACCESS_LOG", "admin-user": "EW_ADMIN_USER", "admin-password": "EW_ADMIN_PASSWORD"}
	for name, env := range values {
		if value := getenv(env); value != "" {
			set(c, name, value)
			sources[fieldName(name)] = "env"
		}
	}
	if value := getenv("EW_TRUSTED_PROXIES"); value != "" {
		c.TrustedProxies = strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' })
		sources["trusted_proxies"] = "env"
	}
}

func Validate(c Config) error {
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("listen: invalid address: %w", err)
	}
	if c.BaseURL == "" || !strings.HasPrefix(c.BaseURL, "/") {
		return errors.New("base_url: must start with /")
	}
	if (c.TLSCert == "") != (c.TLSKey == "") {
		return errors.New("tls_cert and tls_key must be provided together")
	}
	for _, filename := range []string{c.TLSCert, c.TLSKey} {
		if filename != "" {
			if _, err := os.Stat(filename); err != nil {
				return fmt.Errorf("TLS file %q: %w", filename, err)
			}
		}
	}
	for _, value := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(value); err != nil {
			return fmt.Errorf("trusted_proxies: invalid CIDR %q", value)
		}
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return errors.New("log_format: must be text or json")
	}
	if c.AccessLog != "all" && c.AccessLog != "off" && c.AccessLog != "errors-and-writes" {
		return errors.New("access_log: must be all, off, or errors-and-writes")
	}
	return nil
}

func fields(c Config) map[string]any {
	return map[string]any{"listen": c.Listen, "data_dir": c.DataDir, "storage_dir": c.StorageDir, "base_url": c.BaseURL, "tls_cert": c.TLSCert, "tls_key": c.TLSKey, "trusted_proxies": c.TrustedProxies, "log_format": c.LogFormat, "log_level": c.LogLevel, "access_log": c.AccessLog, "admin_user": c.AdminUser, "admin_password": c.AdminPassword}
}
func fieldName(name string) string { return strings.ReplaceAll(name, "-", "_") }
func set(c *Config, name, value string) {
	switch name {
	case "listen":
		c.Listen = value
	case "data-dir":
		c.DataDir = value
	case "storage-dir":
		c.StorageDir = value
	case "base-url":
		c.BaseURL = value
	case "tls-cert":
		c.TLSCert = value
	case "tls-key":
		c.TLSKey = value
	case "log-format":
		c.LogFormat = value
	case "log-level":
		c.LogLevel = value
	case "access-log":
		c.AccessLog = value
	case "admin-user":
		c.AdminUser = value
	case "admin-password":
		c.AdminPassword = value
	}
}
