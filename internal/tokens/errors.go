package tokens

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrExpired      = errors.New("expired")
	ErrInvalidInput = errors.New("invalid input")
	ErrKindMismatch = errors.New("kind mismatch")
)

