package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/canonicaljson"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBStorage struct {
	db    *pgxpool.Pool
	newID func() (uuid.UUID, error)
}

func NewDBStorage(db *pgxpool.Pool) *DBStorage {
	return &DBStorage{db: db, newID: uuid.NewV7}
}

func (s *DBStorage) UploadJSON(ctx context.Context, data any) (*domain.StorageUploadOutput, error) {
	b, err := canonicalJSONBytes(data)
	if err != nil {
		return nil, err
	}

	return s.UploadBytes(ctx, b)
}

func (s *DBStorage) UploadBytes(ctx context.Context, data []byte) (*domain.StorageUploadOutput, error) {
	id, err := s.newID()
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(data)
	contentHash := "0x" + hex.EncodeToString(sum[:])
	txHash := "0x" + hex.EncodeToString(id[:])

	_, err = s.db.Exec(ctx, `
		INSERT INTO storage_contents (id, content_hash, content, created_at)
		VALUES ($1, $2, $3, NOW())
	`, id, contentHash, data)
	if err != nil {
		return nil, err
	}

	return &domain.StorageUploadOutput{TxHash: txHash, RootHash: contentHash}, nil
}

func canonicalJSONBytes(data any) ([]byte, error) {
	return canonicaljson.Bytes(data)
}
