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
	QuickNodeWebhookSecret  string          `yaml:"quicknode_webhook_secret"`
	Database                databaseConfig  `yaml:"database"`
	Contracts               contractsConfig `yaml:"contracts"`
	Payer                   agentConfig     `yaml:"payer"`
	Payee                   agentConfig     `yaml:"payee"`
	WorkOrder               workOrderConfig `yaml:"work_order"`
	DeliveryPayload         map[string]any  `yaml:"delivery_payload"`
	payerRuntime            *agentRuntime
	payeeRuntime            *agentRuntime
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
	RUSD   string `yaml:"rusd"`
	Escrow string `yaml:"escrow"`
}

type agentConfig struct {
	Name        string `yaml:"name"`
	PrivateKey  string `yaml:"private_key"`
	AgentID0G   string `yaml:"agent_id_0g"`
	MetadataCID string `yaml:"metadata_cid"`
}

type workOrderConfig struct {
	Amount             string                     `yaml:"amount"`
	Asset              string                     `yaml:"asset"`
	Chain              string                     `yaml:"chain"`
	Task               workOrderTaskConfig        `yaml:"task"`
	Deliverable        workOrderDeliverableConfig `yaml:"deliverable"`
	AcceptanceCriteria []acceptanceCriteriaConfig `yaml:"acceptance_criteria"`
	Deadline           string                     `yaml:"deadline"`
}

type workOrderTaskConfig struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`
}

type workOrderDeliverableConfig struct {
	Format     string                    `yaml:"format"`
	Submission workOrderSubmissionConfig `yaml:"submission"`
}

type workOrderSubmissionConfig struct {
	Method   string `yaml:"method"`
	Endpoint string `yaml:"endpoint"`
}

type acceptanceCriteriaConfig struct {
	ID               string  `yaml:"id"`
	Description      string  `yaml:"description"`
	VerificationHint *string `yaml:"verification_hint"`
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
	if strings.TrimSpace(c.QuickNodeWebhookSecret) == "" {
		return fmt.Errorf("quicknode_webhook_secret is required")
	}
	if !common.IsHexAddress(c.Contracts.RUSD) {
		return fmt.Errorf("contracts.rusd must be a valid Ethereum address")
	}
	if !common.IsHexAddress(c.Contracts.Escrow) {
		return fmt.Errorf("contracts.escrow must be a valid Ethereum address")
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

	payer, err := validateAgent("payer", c.Payer)
	if err != nil {
		return err
	}
	payee, err := validateAgent("payee", c.Payee)
	if err != nil {
		return err
	}
	if strings.EqualFold(payer.Address.Hex(), payee.Address.Hex()) {
		return fmt.Errorf("payer and payee must derive different wallet addresses")
	}
	c.payerRuntime = payer
	c.payeeRuntime = payee

	if _, err := parsePositiveBigInt("work_order.amount", c.WorkOrder.Amount); err != nil {
		return err
	}
	if strings.TrimSpace(c.WorkOrder.Asset) != "rUSD" {
		return fmt.Errorf("work_order.asset must be rUSD")
	}
	if strings.TrimSpace(c.WorkOrder.Chain) == "" {
		return fmt.Errorf("work_order.chain is required")
	}
	if strings.TrimSpace(c.WorkOrder.Task.Title) == "" {
		return fmt.Errorf("work_order.task.title is required")
	}
	if strings.TrimSpace(c.WorkOrder.Task.Description) == "" {
		return fmt.Errorf("work_order.task.description is required")
	}
	if strings.TrimSpace(c.WorkOrder.Task.Category) == "" {
		return fmt.Errorf("work_order.task.category is required")
	}
	if strings.TrimSpace(c.WorkOrder.Deliverable.Format) == "" {
		return fmt.Errorf("work_order.deliverable.format is required")
	}
	if strings.TrimSpace(c.WorkOrder.Deliverable.Submission.Method) == "" {
		return fmt.Errorf("work_order.deliverable.submission.method is required")
	}
	if strings.TrimSpace(c.WorkOrder.Deliverable.Submission.Endpoint) == "" {
		return fmt.Errorf("work_order.deliverable.submission.endpoint is required")
	}
	if len(c.WorkOrder.AcceptanceCriteria) == 0 {
		return fmt.Errorf("work_order.acceptance_criteria must include at least one item")
	}
	for i, criteria := range c.WorkOrder.AcceptanceCriteria {
		if strings.TrimSpace(criteria.ID) == "" {
			return fmt.Errorf("work_order.acceptance_criteria[%d].id is required", i)
		}
		if strings.TrimSpace(criteria.Description) == "" {
			return fmt.Errorf("work_order.acceptance_criteria[%d].description is required", i)
		}
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(c.WorkOrder.Deadline)); err != nil {
		return fmt.Errorf("work_order.deadline must be a valid RFC3339 timestamp")
	}
	if len(c.DeliveryPayload) == 0 {
		return fmt.Errorf("delivery_payload is required")
	}

	return nil
}

func validateAgent(field string, agent agentConfig) (*agentRuntime, error) {
	name := strings.TrimSpace(agent.Name)
	if name == "" {
		return nil, fmt.Errorf("%s.name is required", field)
	}
	key := strings.TrimSpace(agent.PrivateKey)
	privateKey, err := crypto.HexToECDSA(trimHexPrefix(key))
	if err != nil {
		return nil, fmt.Errorf("%s.private_key is invalid: %w", field, err)
	}
	if strings.TrimSpace(agent.AgentID0G) == "" {
		agent.AgentID0G = "rive-demo-escrow-" + name
	}
	if strings.TrimSpace(agent.MetadataCID) == "" {
		agent.MetadataCID = "demo-agent-escrow-" + name
	}

	return &agentRuntime{
		Config:     agent,
		Address:    crypto.PubkeyToAddress(privateKey.PublicKey),
		PrivateKey: key,
	}, nil
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
