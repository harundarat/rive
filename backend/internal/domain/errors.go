package domain

import "errors"

var (
	ErrNotFound    = errors.New("not found")
	ErrStorage     = errors.New("storage error")
	ErrPersistence = errors.New("persistence error")
)
