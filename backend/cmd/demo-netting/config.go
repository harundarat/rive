package main

import (
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.yaml.in/yaml/v3"
)

const (
	defaultPollTimeout = 90 * time.Second
	defaultDecimals    = 18
)

type demoConfig struct {
	APIBaseURL              string          `yaml:"api_base_url"`
	RPCURL                  string          `yaml:"rpc_url"`
	ExplorerTxBaseURL       string          `yaml:"explorer_tx_base_url"`
	PollTimeoutSeconds      int             `yaml:"poll_timeout_seconds"`
	TokenDecimals           int             `yaml:"token_decimals"`
	MinimumNativeBalanceWei string          `yaml:"minimum_native_balance_wei"`
	MintBufferAmount        string          `yaml:"mint_buffer_amount"`
	Database                databaseConfig  `yaml:"database"`
	Contracts               contractsConfig `yaml:"contracts"`
	Agents                  []agentConfig   `yaml:"agents"`
	Intents                 []intentConfig  `yaml:"intents"`
	agentByName             map[string]*agentRuntime
}

type databaseConfig struct {
	DSN         string `yaml:"dsn"`
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	Name        string `yaml:"name"`
	SSLMode     string `yaml:"sslmode"`
	SSLRootCert string `yaml:"sslrootcert"`
}

type contractsConfig struct {
	RUSD              string `yaml:"rusd"`
	NettingSettlement string `yaml:"netting_settlement"`
}

type agentConfig struct {
	Name        string `yaml:"name"`
	PrivateKey  string `yaml:"private_key"`
	AgentID0G   string `yaml:"agent_id_0g"`
	MetadataCID string `yaml:"metadata_cid"`
}

type intentConfig struct {
	Payer  string `yaml:"payer"`
	Payee  string `yaml:"payee"`
	Amount string `yaml:"amount"`
}

type agentRuntime struct {
	Config     agentConfig
	Address    common.Address
	PrivateKey string
}

func loadDemoConfig(path string) (*demoConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg demoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *demoConfig) validate() error {
	c.APIBaseURL = strings.TrimRight(strings.TrimSpace(c.APIBaseURL), "/")
	if c.APIBaseURL == "" {
		return fmt.Errorf("api_base_url is required")
	}
	if strings.TrimSpace(c.RPCURL) == "" {
		return fmt.Errorf("rpc_url is required")
	}
	if !common.IsHexAddress(c.Contracts.RUSD) {
		return fmt.Errorf("contracts.rusd must be a valid Ethereum address")
	}
	if !common.IsHexAddress(c.Contracts.NettingSettlement) {
		return fmt.Errorf("contracts.netting_settlement must be a valid Ethereum address")
	}
	if strings.TrimSpace(c.Database.DSN) == "" && (strings.TrimSpace(c.Database.Host) == "" || strings.TrimSpace(c.Database.User) == "" || strings.TrimSpace(c.Database.Name) == "") {
		return fmt.Errorf("database dsn or host/user/name is required")
	}
	if c.Database.Port == 0 {
		c.Database.Port = 5432
	}
	if strings.TrimSpace(c.Database.SSLMode) == "" {
		c.Database.SSLMode = "disable"
	}
	if c.PollTimeoutSeconds <= 0 {
		c.PollTimeoutSeconds = int(defaultPollTimeout.Seconds())
	}
	if c.TokenDecimals <= 0 {
		c.TokenDecimals = defaultDecimals
	}
	if strings.TrimSpace(c.MinimumNativeBalanceWei) == "" {
		c.MinimumNativeBalanceWei = "0"
	}
	if _, err := parsePositiveOrZeroBigInt("minimum_native_balance_wei", c.MinimumNativeBalanceWei); err != nil {
		return err
	}
	if strings.TrimSpace(c.MintBufferAmount) == "" {
		c.MintBufferAmount = "0"
	}
	if _, err := parsePositiveOrZeroBigInt("mint_buffer_amount", c.MintBufferAmount); err != nil {
		return err
	}
	if len(c.Agents) < 2 {
		return fmt.Errorf("agents must include at least two entries")
	}
	if len(c.Intents) == 0 {
		return fmt.Errorf("intents must include at least one entry")
	}

	c.agentByName = map[string]*agentRuntime{}
	seenAddresses := map[string]string{}
	for i, agent := range c.Agents {
		name := strings.TrimSpace(agent.Name)
		if name == "" {
			return fmt.Errorf("agents[%d].name is required", i)
		}
		if _, exists := c.agentByName[name]; exists {
			return fmt.Errorf("duplicate agent name %q", name)
		}
		key := strings.TrimSpace(agent.PrivateKey)
		privateKey, err := crypto.HexToECDSA(trimHexPrefix(key))
		if err != nil {
			return fmt.Errorf("agents[%d].private_key is invalid: %w", i, err)
		}
		address := crypto.PubkeyToAddress(privateKey.PublicKey)
		if existingName, exists := seenAddresses[strings.ToLower(address.Hex())]; exists {
			return fmt.Errorf("agents %q and %q derive the same address", existingName, name)
		}
		seenAddresses[strings.ToLower(address.Hex())] = name
		if strings.TrimSpace(agent.AgentID0G) == "" {
			agent.AgentID0G = "rive-demo-" + name
		}
		if strings.TrimSpace(agent.MetadataCID) == "" {
			agent.MetadataCID = "demo-agent-" + name
		}

		c.agentByName[name] = &agentRuntime{
			Config:     agent,
			Address:    address,
			PrivateKey: key,
		}
	}

	for i, intent := range c.Intents {
		if _, ok := c.agentByName[strings.TrimSpace(intent.Payer)]; !ok {
			return fmt.Errorf("intents[%d].payer references unknown agent %q", i, intent.Payer)
		}
		if _, ok := c.agentByName[strings.TrimSpace(intent.Payee)]; !ok {
			return fmt.Errorf("intents[%d].payee references unknown agent %q", i, intent.Payee)
		}
		if strings.TrimSpace(intent.Payer) == strings.TrimSpace(intent.Payee) {
			return fmt.Errorf("intents[%d] payer and payee must be different", i)
		}
		if _, err := parsePositiveBigInt(fmt.Sprintf("intents[%d].amount", i), intent.Amount); err != nil {
			return err
		}
	}

	return nil
}

func (c databaseConfig) dsn() string {
	if strings.TrimSpace(c.DSN) != "" {
		return c.DSN
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   "/" + c.Name,
	}
	if c.Password == "" {
		u.User = url.User(c.User)
	} else {
		u.User = url.UserPassword(c.User, c.Password)
	}

	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	if strings.TrimSpace(c.SSLRootCert) != "" {
		q.Set("sslrootcert", c.SSLRootCert)
	}
	u.RawQuery = q.Encode()

	return u.String()
}

func parsePositiveBigInt(name string, value string) (*big.Int, error) {
	amount, err := parsePositiveOrZeroBigInt(name, value)
	if err != nil {
		return nil, err
	}
	if amount.Sign() <= 0 {
		return nil, fmt.Errorf("%s must be greater than zero", name)
	}

	return amount, nil
}

func parsePositiveOrZeroBigInt(name string, value string) (*big.Int, error) {
	amount := new(big.Int)
	if _, ok := amount.SetString(strings.TrimSpace(value), 10); !ok || amount.Sign() < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer", name)
	}

	return amount, nil
}

func trimHexPrefix(value string) string {
	return strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
}
