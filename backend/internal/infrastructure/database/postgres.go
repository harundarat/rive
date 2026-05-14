package database

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"strings"

	"github.com/harundarat/rive/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	if strings.TrimSpace(cfg.SSLCACert) != "" {
		return openWithInlineCACert(cfg)
	}

	dbpool, err := pgxpool.New(context.Background(), cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	log.Println("Database connected")
	return dbpool, nil
}

// openWithInlineCACert builds the connection pool using a CA certificate provided
// as PEM content in DB_SSL_CA, avoiding any filesystem dependency.
func openWithInlineCACert(cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM([]byte(cfg.SSLCACert)) {
		return nil, fmt.Errorf("DB_SSL_CA: no valid PEM certificate found")
	}

	if poolCfg.ConnConfig.TLSConfig == nil {
		poolCfg.ConnConfig.TLSConfig = &tls.Config{}
	}
	poolCfg.ConnConfig.TLSConfig.RootCAs = certPool

	dbpool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	log.Println("Database connected")
	return dbpool, nil
}
