package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type batchResult struct {
	ID                      uuid.UUID
	Status                  string
	SettlementTxHash        *string
	GrossIntentCount        int
	SettlementTransferCount int
	GrossAmount             *big.Int
	NetAmount               *big.Int
}

type batchSettlementPollResult struct {
	batch *batchResult
	err   error
}

func openDB(ctx context.Context, cfg *demoConfig) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, cfg.Database.dsn())
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}

func seedAgents(ctx context.Context, db *pgxpool.Pool, cfg *demoConfig) error {
	for _, agent := range cfg.agentByName {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generate agent id: %w", err)
		}
		_, err = db.Exec(ctx, `
			INSERT INTO agents (
				id,
				agent_id_0g,
				wallet_address,
				reputation_score,
				metadata_cid,
				created_at
			) VALUES ($1, $2, $3, 0, $4, NOW())
			ON CONFLICT (wallet_address) DO UPDATE
			SET
				agent_id_0g = EXCLUDED.agent_id_0g,
				metadata_cid = EXCLUDED.metadata_cid
		`,
			id,
			agent.Config.AgentID0G,
			agent.Address.Hex(),
			agent.Config.MetadataCID,
		)
		if err != nil {
			return fmt.Errorf("upsert agent %s: %w", agent.Config.Name, err)
		}
	}

	return nil
}

func ensureNoPendingNetting(ctx context.Context, db *pgxpool.Pool) error {
	var count int64
	err := db.QueryRow(ctx, `
		SELECT COUNT(*)::bigint
		FROM payment_intents
		WHERE status IN ('pending', 'batched')
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("count existing pending payment intents: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("found %d existing pending/batched payment intents; clear or settle them before demo to avoid mixed batches", count)
	}

	return nil
}

func pollBatchSettlementWithCountdown(ctx context.Context, db *pgxpool.Pool, runID string, expectedIntentCount int, timeout time.Duration) (*batchResult, error) {
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultCh := make(chan batchSettlementPollResult, 1)
	go func() {
		batch, err := pollBatchSettlement(pollCtx, db, runID, expectedIntentCount, timeout)
		resultCh <- batchSettlementPollResult{batch: batch, err: err}
	}()

	startedAt := time.Now()
	maxLineLen := renderBatchSettlementCountdown(startedAt, timeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case result := <-resultCh:
			clearBatchSettlementCountdown(maxLineLen)
			return result.batch, result.err
		case <-ticker.C:
			lineLen := renderBatchSettlementCountdown(startedAt, timeout)
			if lineLen > maxLineLen {
				maxLineLen = lineLen
			}
		case <-ctx.Done():
			clearBatchSettlementCountdown(maxLineLen)
			return nil, ctx.Err()
		}
	}
}

func renderBatchSettlementCountdown(startedAt time.Time, timeout time.Duration) int {
	line := batchSettlementCountdownLine(startedAt, timeout)
	fmt.Printf("\r%s", line)
	return len(line)
}

func clearBatchSettlementCountdown(lineLen int) {
	if lineLen <= 0 {
		fmt.Print("\r")
		return
	}

	fmt.Printf("\r%s\r", strings.Repeat(" ", lineLen))
}

func batchSettlementCountdownLine(startedAt time.Time, timeout time.Duration) string {
	const barWidth = 12

	elapsed := time.Since(startedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > timeout {
		elapsed = timeout
	}

	filled := 0
	if timeout > 0 {
		filled = int(float64(elapsed) / float64(timeout) * float64(barWidth))
	}
	if filled < 0 {
		filled = 0
	}
	if filled > barWidth {
		filled = barWidth
	}

	return fmt.Sprintf(
		"Waiting for batch settlement [%s%s] %s / %s",
		strings.Repeat("#", filled),
		strings.Repeat(".", barWidth-filled),
		formatCountdownDuration(elapsed),
		formatCountdownDuration(timeout),
	)
}

func formatCountdownDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	totalSeconds := int(duration / time.Second)
	return fmt.Sprintf("%d:%02d", totalSeconds/60, totalSeconds%60)
}

func pollBatchSettlement(ctx context.Context, db *pgxpool.Pool, runID string, expectedIntentCount int, timeout time.Duration) (*batchResult, error) {
	deadline := time.Now().Add(timeout)
	pattern := idempotencyPrefix(runID) + "%"

	for {
		var total int
		var settled int
		var failed int
		err := db.QueryRow(ctx, `
			SELECT
				COUNT(*)::int,
				COUNT(*) FILTER (WHERE status = 'settled')::int,
				COUNT(*) FILTER (WHERE status = 'failed')::int
			FROM payment_intents
			WHERE idempotency_key LIKE $1
		`, pattern).Scan(&total, &settled, &failed)
		if err != nil {
			return nil, fmt.Errorf("poll payment intent status: %w", err)
		}
		if failed > 0 {
			return nil, failureDetails(ctx, db, pattern)
		}
		if total == expectedIntentCount && settled == expectedIntentCount {
			batch, err := fetchRunBatch(ctx, db, pattern)
			if err != nil {
				return nil, err
			}
			if batch.Status == "settled" {
				return batch, nil
			}
			if batch.Status == "failed" {
				return nil, fmt.Errorf("netting batch %s failed", batch.ID)
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for netting settlement: total=%d settled=%d expected=%d", total, settled, expectedIntentCount)
		}

		time.Sleep(2 * time.Second)
	}
}

func fetchRunBatch(ctx context.Context, db *pgxpool.Pool, idempotencyPattern string) (*batchResult, error) {
	var result batchResult
	var txHash pgtype.Text
	var grossText string
	var netText string
	err := db.QueryRow(ctx, `
		SELECT
			netting_batches.id,
			netting_batches.batch_status,
			netting_batches.settlement_tx_hash,
			netting_batches.gross_intent_count,
			netting_batches.settlement_transfer_count,
			netting_batches.gross_amount::text,
			netting_batches.net_amount::text
		FROM netting_batches
		INNER JOIN payment_intents ON payment_intents.netting_batch_id = netting_batches.id
		WHERE payment_intents.idempotency_key LIKE $1
		GROUP BY
			netting_batches.id,
			netting_batches.batch_status,
			netting_batches.settlement_tx_hash,
			netting_batches.gross_intent_count,
			netting_batches.settlement_transfer_count,
			netting_batches.gross_amount,
			netting_batches.net_amount,
			netting_batches.updated_at
		ORDER BY netting_batches.updated_at DESC
		LIMIT 1
	`, idempotencyPattern).Scan(
		&result.ID,
		&result.Status,
		&txHash,
		&result.GrossIntentCount,
		&result.SettlementTransferCount,
		&grossText,
		&netText,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no netting batch found for run")
	}
	if err != nil {
		return nil, fmt.Errorf("fetch netting batch: %w", err)
	}
	if txHash.Valid && strings.TrimSpace(txHash.String) != "" {
		result.SettlementTxHash = &txHash.String
	}
	gross, ok := new(big.Int).SetString(grossText, 10)
	if !ok {
		return nil, fmt.Errorf("invalid gross_amount from database: %q", grossText)
	}
	net, ok := new(big.Int).SetString(netText, 10)
	if !ok {
		return nil, fmt.Errorf("invalid net_amount from database: %q", netText)
	}
	result.GrossAmount = gross
	result.NetAmount = net

	return &result, nil
}

func failureDetails(ctx context.Context, db *pgxpool.Pool, idempotencyPattern string) error {
	rows, err := db.Query(ctx, `
		SELECT idempotency_key, COALESCE(failure_reason, '')
		FROM payment_intents
		WHERE idempotency_key LIKE $1
			AND status = 'failed'
		ORDER BY idempotency_key
	`, idempotencyPattern)
	if err != nil {
		return fmt.Errorf("fetch failure details: %w", err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var key string
		var reason string
		if err := rows.Scan(&key, &reason); err != nil {
			return fmt.Errorf("scan failure details: %w", err)
		}
		details = append(details, fmt.Sprintf("%s: %s", key, reason))
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate failure details: %w", err)
	}
	if len(details) == 0 {
		return fmt.Errorf("one or more payment intents failed")
	}

	return fmt.Errorf("payment intents failed: %s", strings.Join(details, "; "))
}

func idempotencyPrefix(runID string) string {
	return "netting-demo-" + runID + "-"
}
