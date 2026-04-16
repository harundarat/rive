package domain

import (
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID              uuid.UUID `json:"id"`
	AgentID0G       string    `json:"agent_id_0g"`
	WalletAddress   string    `json:"wallet_address"`
	ReputationScore float64   `json:"reputation_score"`
	MetadataCID     string    `json:"metadata_cid"`
	CreatedAt       time.Time `json:"created_at"`
}
