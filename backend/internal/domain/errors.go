package domain

import "errors"

var (
	ErrNotFound              = errors.New("not found")
	ErrStorage               = errors.New("storage error")
	ErrPersistence           = errors.New("persistence error")
	ErrInvalidSignature      = errors.New("invalid signature")
	ErrPayeeMismatch         = errors.New("payee mismatch")
	ErrOrderNotFunded        = errors.New("order not funded")
	ErrDeliveryAlreadyPosted = errors.New("delivery already posted")
	ErrUnsupportedAsset      = errors.New("unsupported asset")
	ErrSettlement            = errors.New("settlement error")
)
