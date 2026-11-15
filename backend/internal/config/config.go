// Package config loads runtime configuration from environment variables.
package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port         string
	MongoURI     string
	MongoDB      string
	JWTSecret    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	InviteTTL    time.Duration
	CookieSecure bool
	CORSOrigin   string
	// AppBaseURL is the web application's address, used to build links in emails.
	AppBaseURL string
	// WebhookURL is the n8n webhook that receives events. Empty disables delivery.
	WebhookURL    string
	WebhookSecret string
}

// Load reads .env (if present) and then the process environment.
func Load() (Config, error) {
	loadDotEnv(".env")
	c := Config{
		Port:          get("PORT", "8080"),
		MongoURI:      get("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:       get("MONGO_DB", "peoplepulse"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		AccessTTL:     dur("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTTL:    dur("REFRESH_TOKEN_TTL", 7*24*time.Hour),
		InviteTTL:     dur("INVITE_TTL", 72*time.Hour),
		CookieSecure:  get("COOKIE_SECURE", "true") != "false",
		CORSOrigin:    get("CORS_ORIGIN", "http://localhost:3000"),
		AppBaseURL:    strings.TrimRight(get("APP_BASE_URL", "http://localhost:3000"), "/"),
		WebhookURL:    os.Getenv("N8N_WEBHOOK_URL"),
		WebhookSecret: os.Getenv("N8N_WEBHOOK_SECRET"),
	}
	if c.WebhookURL != "" && len(c.WebhookSecret) < 32 {
		return c, errors.New("N8N_WEBHOOK_SECRET must be at least 32 characters when N8N_WEBHOOK_URL is set")
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be set and at least 32 characters long")
	}
	return c, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// loadDotEnv sets KEY=VALUE pairs from a file without overriding real env vars.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, set := os.LookupEnv(k); !set {
			os.Setenv(k, v)
		}
	}
}
