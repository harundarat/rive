package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/harundarat/rive/backend/internal/domain"
)

type fakeStorage struct {
	jsonData any
	byteData []byte
	err      error
	calls    int
}

func (s *fakeStorage) UploadJSON(ctx context.Context, data any) (*domain.StorageUploadOutput, error) {
	s.calls++
	s.jsonData = data
	if s.err != nil {
		return nil, s.err
	}

	return &domain.StorageUploadOutput{TxHash: "0xtx", RootHash: "0xroot"}, nil
}

func (s *fakeStorage) UploadBytes(ctx context.Context, data []byte) (*domain.StorageUploadOutput, error) {
	s.calls++
	s.byteData = append([]byte(nil), data...)
	if s.err != nil {
		return nil, s.err
	}

	return &domain.StorageUploadOutput{TxHash: "0xtx", RootHash: "0xroot"}, nil
}

func TestStorageHandlerUploadCanonicalizesJSON(t *testing.T) {
	storage := &fakeStorage{}
	handler := NewStorageHandler(storage)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", bytes.NewBufferString(`{"z":2,"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if string(storage.byteData) != `{"a":1,"z":2}` {
		t.Fatalf("expected canonical JSON bytes, got %s", string(storage.byteData))
	}

	var body struct {
		Success bool                       `json:"success"`
		Data    domain.StorageUploadOutput `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !body.Success {
		t.Fatal("expected success response")
	}
	if body.Data.RootHash != "0xroot" || body.Data.TxHash != "0xtx" {
		t.Fatalf("unexpected upload output: %+v", body.Data)
	}
}

func TestStorageHandlerUploadRejectsInvalidJSON(t *testing.T) {
	storage := &fakeStorage{}
	handler := NewStorageHandler(storage)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if storage.calls != 0 {
		t.Fatalf("expected no upload calls, got %d", storage.calls)
	}
}

func TestStorageHandlerUploadRejectsTrailingJSONTokens(t *testing.T) {
	storage := &fakeStorage{}
	handler := NewStorageHandler(storage)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", bytes.NewBufferString(`{"a":1} {"b":2}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if storage.calls != 0 {
		t.Fatalf("expected no upload calls, got %d", storage.calls)
	}
}

func TestStorageHandlerUploadRawBlob(t *testing.T) {
	storage := &fakeStorage{}
	handler := NewStorageHandler(storage)
	payload := []byte{0, 1, 2, 'r', 'i', 'v', 'e'}
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(storage.byteData, payload) {
		t.Fatalf("expected raw blob bytes %v, got %v", payload, storage.byteData)
	}
}

func TestStorageHandlerUploadRejectsEmptyBody(t *testing.T) {
	storage := &fakeStorage{}
	handler := NewStorageHandler(storage)
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if storage.calls != 0 {
		t.Fatalf("expected no upload calls, got %d", storage.calls)
	}
}

func TestStorageHandlerUploadStorageError(t *testing.T) {
	handler := NewStorageHandler(&fakeStorage{err: errors.New("upload failed")})
	req := httptest.NewRequest(nethttp.MethodPost, "/api/storage/upload", bytes.NewBufferString(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.Upload(rec, req)

	if rec.Code != nethttp.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Error.Code != "FAILED_TO_UPLOAD_TO_STORAGE" {
		t.Fatalf("expected storage error code, got %q", body.Error.Code)
	}
}
