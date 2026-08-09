package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://kanvas:secret@db/kanvas")
	t.Setenv(EnvConfluenceDSN, "legacy:secret@tcp(mysql:3306)/confluence")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvBootstrapSecret, "correct-horse-battery-staple")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenAddress != ":8080" || c.DataDir == "" {
		t.Fatalf("unexpected defaults: %#v", c)
	}
}

func TestLoadRejectsWeakBootstrapPassword(t *testing.T) {
	t.Setenv(EnvPostgresDSN, "postgres://db/kanvas")
	t.Setenv(EnvBootstrapAdmin, "admin")
	t.Setenv(EnvBootstrapSecret, "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected weak password error")
	}
}
