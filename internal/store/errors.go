package store

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrNameConflict       = errors.New("name conflict")
	ErrTooLarge           = errors.New("too large")
	ErrInsufficientSpace  = errors.New("insufficient space")
	ErrInvalidInput       = errors.New("invalid input")
	ErrReplaceWouldExceed = errors.New("replace would exceed limits")
)

