package response

import (
	"encoding/json"
	"net/http"

	"github.com/harundarat/rive/backend/pkg/apierror"
)

type Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	TotalItems int64 `json:"total_items"`
	TotalPages int   `json:"total_pages"`
}

type Envelope[T any] struct {
	Success bool               `json:"success"`
	Data    T                  `json:"data,omitempty"`
	Error   *apierror.APIError `json:"error,omitempty"`
	Meta    *Meta              `json:"meta,omitempty"`
}

func JSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(body)
}

func Success[T any](w http.ResponseWriter, statusCode int, data T) {
	JSON(w, statusCode, Envelope[T]{Success: true, Data: data})
}

func Paginated[T any](w http.ResponseWriter, statusCode int, data T, meta Meta) {
	JSON(w, statusCode, Envelope[T]{Success: true, Data: data, Meta: &meta})
}

func Error(w http.ResponseWriter, err *apierror.APIError) {
	JSON(w, err.HTTPStatus, Envelope[any]{Success: false, Error: err})
}
