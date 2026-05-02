package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

type DatabaseConfig struct {
	Host         string `mapstructure:"DB_HOST"`
	Port         int    `mapstructure:"DB_PORT"`
	User         string `mapstructure:"DB_USER"`
	Password     string `mapstructure:"DB_PASSWORD"`
	Name         string `mapstructure:"DB_NAME"`
	SSLMode      string `mapstructure:"DB_SSLMODE"`
	SSLRootCert  string `mapstructure:"DB_SSLROOTCERT"`
	MaxIdleConns int    `mapstructure:"DB_MAX_IDLE_CONN"`
	MaxOpenConns int    `mapstructure:"DB_MAX_OPEN_CONN"`
}

func (d *DatabaseConfig) DSN() string {
	return d.buildURL("postgres")
}

func (d *DatabaseConfig) buildURL(scheme string) string {
	u := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:   "/" + d.Name,
	}

	if d.Password == "" {
		u.User = url.User(d.User)
	} else {
		u.User = url.UserPassword(d.User, d.Password)
	}

	q := u.Query()
	q.Set("sslmode", d.SSLMode)
	if sslRootCert := strings.TrimSpace(d.SSLRootCert); sslRootCert != "" {
		q.Set("sslrootcert", sslRootCert)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

type ZeroGStorageConfig struct {
	EVMRPC     string `mapstructure:"ZG_EVM_RPC"`
	IndexerRPC string `mapstructure:"ZG_STORAGE_INDEXER_RPC"`
	PrivateKey string `mapstructure:"ZG_PRIVATE_KEY"`
}

type NettingConfig struct {
	SettlementAddress string `mapstructure:"NETTING_SETTLEMENT_ADDRESS"`
	SettlerPrivateKey string `mapstructure:"NETTING_SETTLER_PRIVATE_KEY"`
	WindowSeconds     int    `mapstructure:"NETTING_WINDOW_SECONDS"`
}

type Config struct {
	Database               DatabaseConfig     `mapstructure:",squash"`
	ZeroG                  ZeroGStorageConfig `mapstructure:",squash"`
	Netting                NettingConfig      `mapstructure:",squash"`
	QuickNodeWebhookSecret string             `mapstructure:"QUICKNODE_WEBHOOK_SECRET"`
	EscrowContractAddress  string             `mapstructure:"ESCROW_CONTRACT_ADDRESS"`
}

func Load() (*Config, error) {
	viper.SetDefault("NETTING_WINDOW_SECONDS", 60)

	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}

	var config Config
	err = viper.Unmarshal(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}
