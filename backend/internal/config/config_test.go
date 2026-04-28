package config

import (
	"net/url"
	"testing"
)

func TestDatabaseConfigDSNIncludesSSLMode(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "pg.example.com",
		Port:     5432,
		User:     "avnadmin",
		Password: "secret",
		Name:     "rive",
		SSLMode:  "require",
	}

	parsed, err := url.Parse(cfg.DSN())
	if err != nil {
		t.Fatalf("failed to parse DSN: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("sslmode"); got != "require" {
		t.Fatalf("expected sslmode=require, got %q", got)
	}
	if query.Has("sslrootcert") {
		t.Fatalf("expected no sslrootcert when unset, got %q", query.Get("sslrootcert"))
	}
}

func TestDatabaseConfigDSNIncludesSSLRootCertWhenSet(t *testing.T) {
	const sslRootCert = `C:\Users\harun\certs\aiven ca.pem`
	cfg := DatabaseConfig{
		Host:        "pg.example.com",
		Port:        5432,
		User:        "avnadmin",
		Password:    "secret",
		Name:        "rive",
		SSLMode:     "verify-full",
		SSLRootCert: sslRootCert,
	}

	parsed, err := url.Parse(cfg.DSN())
	if err != nil {
		t.Fatalf("failed to parse DSN: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("sslmode"); got != "verify-full" {
		t.Fatalf("expected sslmode=verify-full, got %q", got)
	}
	if got := query.Get("sslrootcert"); got != sslRootCert {
		t.Fatalf("expected sslrootcert %q, got %q", sslRootCert, got)
	}
	if parsed.RawQuery == query.Encode() {
		return
	}
	t.Fatalf("expected raw query to be URL encoded consistently, got %q", parsed.RawQuery)
}

func TestDatabaseConfigDSNTrimsBlankSSLRootCert(t *testing.T) {
	cfg := DatabaseConfig{
		Host:        "pg.example.com",
		Port:        5432,
		User:        "avnadmin",
		Password:    "secret",
		Name:        "rive",
		SSLMode:     "require",
		SSLRootCert: "   ",
	}

	parsed, err := url.Parse(cfg.DSN())
	if err != nil {
		t.Fatalf("failed to parse DSN: %v", err)
	}

	if parsed.Query().Has("sslrootcert") {
		t.Fatalf("expected blank sslrootcert to be omitted, got %q", parsed.Query().Get("sslrootcert"))
	}
}
