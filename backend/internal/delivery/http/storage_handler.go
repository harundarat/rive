package http

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/harundarat/rive/backend/internal/domain"
	"github.com/harundarat/rive/backend/pkg/apierror"
	"github.com/harundarat/rive/backend/pkg/canonicaljson"
	"github.com/harundarat/rive/backend/pkg/response"
)

type StorageHandler struct {
	zgStorage domain.ZGStorage
}

func NewStorageHandler(zgStorage domain.ZGStorage) *StorageHandler {
	return &StorageHandler{zgStorage: zgStorage}
}

func (h *StorageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if isJSONContentType(r.Header.Get("Content-Type")) {
		h.uploadJSON(w, r)
		return
	}

	h.uploadBytes(w, r)
}

func (h *StorageHandler) uploadJSON(w http.ResponseWriter, r *http.Request) {
	var payload any
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&payload); err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	data, err := canonicaljson.Bytes(payload)
	if err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}

	output, err := h.zgStorage.UploadBytes(r.Context(), data)
	if err != nil {
		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_UPLOAD_TO_0G_STORAGE", err.Error()))
		return
	}

	response.Success(w, http.StatusCreated, output)
}

func (h *StorageHandler) uploadBytes(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "invalid request payload"))
		return
	}
	if len(data) == 0 {
		response.Error(w, apierror.New(http.StatusBadRequest, "BAD_REQUEST", "request body is required"))
		return
	}

	output, err := h.zgStorage.UploadBytes(r.Context(), data)
	if err != nil {
		response.Error(w, apierror.New(http.StatusInternalServerError, "FAILED_TO_UPLOAD_TO_0G_STORAGE", err.Error()))
		return
	}

	response.Success(w, http.StatusCreated, output)
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	}
	mediaType = strings.ToLower(mediaType)

	return mediaType == "application/json" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}
