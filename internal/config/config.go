package config

import (
	"errors"
	"os"
	"strings"
)

const (
	EnvPostgresDSN     = "KANVAS_POSTGRES_DSN"
	EnvConfluenceDSN   = "KANVAS_CONFLUENCE_DSN"
	EnvBootstrapAdmin  = "KANVAS_BOOTSTRAP_ADMIN"
	EnvBootstrapSecret = "KANVAS_BOOTSTRAP_ADMIN_PASSWORD"
)

type Config struct {
	PostgresDSN       string
	ConfluenceDSN     string
	BootstrapAdmin    string
	BootstrapPassword string
	ListenAddress     string
	DataDir           string
}

func Load() (Config, error) {
	c := Config{
		PostgresDSN:       strings.TrimSpace(os.Getenv(EnvPostgresDSN)),
		ConfluenceDSN:     strings.TrimSpace(os.Getenv(EnvConfluenceDSN)),
		BootstrapAdmin:    strings.TrimSpace(os.Getenv(EnvBootstrapAdmin)),
		BootstrapPassword: os.Getenv(EnvBootstrapSecret),
		ListenAddress:     ":8080",
		DataDir:           "/var/lib/kanvas",
	}
	if c.PostgresDSN == "" {
		return Config{}, errors.New(EnvPostgresDSN + " is required")
	}
	if c.BootstrapAdmin == "" {
		return Config{}, errors.New(EnvBootstrapAdmin + " is required")
	}
	if len(c.BootstrapPassword) < 12 {
		return Config{}, errors.New(EnvBootstrapSecret + " must be at least 12 characters")
	}
	return c, nil
}
