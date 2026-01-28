package text

import "errors"

var (
	ErrNotText             = errors.New("not text")
	ErrUnsupportedEncoding = errors.New("unsupported encoding")
	ErrUnknownSource       = errors.New("unknown source encoding")
	ErrDecodeFailed        = errors.New("decode failed")
	ErrEncodeFailed        = errors.New("encode failed")
	ErrUnrepresentable     = errors.New("unrepresentable in target encoding")
	ErrInvalidInput        = errors.New("invalid input")
)

