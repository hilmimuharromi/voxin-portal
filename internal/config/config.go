// Package config reads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	PublicAddr    string // public site listener, e.g. :8090 (voxin.id)
	CMSAddr       string // newsroom CMS listener, e.g. :8091 (cms.voxin.id)
	DatabaseURL   string
	DataDir       string // uploaded media
	SecretKey     string // session cookie signing
	AdminEmail    string // first account, created when there are no users
	AdminPassword string
	BaseURL       string // public base URL, e.g. https://voxin.id
	CMSURL        string // CMS base URL, e.g. https://cms.voxin.id
	Site          string // site key served on the public listener
	TZ            string
	SecureCookie  bool
}

func Load() (*Config, error) {
	c := &Config{
		PublicAddr:    env("PORTAL_PUBLIC_ADDR", ":8090"),
		CMSAddr:       env("PORTAL_CMS_ADDR", ":8091"),
		DatabaseURL:   env("DATABASE_URL", "postgres://voxin:voxin@localhost:5432/voxin_portal?sslmode=disable"),
		DataDir:       env("PORTAL_DATA_DIR", "./data"),
		SecretKey:     os.Getenv("PORTAL_SECRET_KEY"),
		AdminEmail:    strings.ToLower(strings.TrimSpace(os.Getenv("PORTAL_ADMIN_EMAIL"))),
		AdminPassword: os.Getenv("PORTAL_ADMIN_PASSWORD"),
		BaseURL:       strings.TrimRight(env("PORTAL_BASE_URL", "http://localhost:8090"), "/"),
		CMSURL:        strings.TrimRight(env("PORTAL_CMS_URL", "http://localhost:8091"), "/"),
		Site:          env("PORTAL_SITE", "voxin"),
		TZ:            env("TZ", "Asia/Jakarta"),
	}
	c.SecureCookie = strings.HasPrefix(c.CMSURL, "https://")
	if len(c.SecretKey) < 16 {
		return nil, errors.New("PORTAL_SECRET_KEY wajib diisi, minimal 16 karakter (lihat .env.example)")
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return nil, err
	}
	c.DataDir = abs
	if err := os.MkdirAll(filepath.Join(c.DataDir, "media"), 0o755); err != nil {
		return nil, fmt.Errorf("buat folder data: %w", err)
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
