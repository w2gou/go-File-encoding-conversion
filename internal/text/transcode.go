package text

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

type TranscodeParams struct {
	SourceEncoding string
	TargetEncoding string
}

// StrictTranscode 严格转码：
// - 仅对可识别文本可用（自动模式会先做保守识别，否则直接失败）
// - 任一步解码/编码失败直接返回错误
// - 不做“替换字符/容错写回”
func StrictTranscode(src []byte, p TranscodeParams) ([]byte, string, error) {
	if p.TargetEncoding == "" {
		return nil, "", fmt.Errorf("%w: target encoding required", ErrInvalidInput)
	}

	sourceEnc := p.SourceEncoding
	if sourceEnc == "" {
		sourceEnc = SourceEncodingAuto
	}

	if sourceEnc == SourceEncodingAuto {
		isText, enc := DetectTextAndEncoding(src)
		if !isText || enc == EncodingUnknown {
			return nil, "", ErrNotText
		}
		sourceEnc = enc
	}

	utf8Bytes, err := decodeStrictBytes(sourceEnc, src)
	if err != nil {
		if err == ErrUnsupportedEncoding {
			return nil, "", err
		}
		if err == ErrDecodeFailed {
			return nil, "", err
		}
		return nil, "", ErrDecodeFailed
	}
	if !utf8.Valid(utf8Bytes) {
		return nil, "", ErrDecodeFailed
	}

	out, err := encodeStrictBytes(p.TargetEncoding, utf8Bytes)
	if err != nil {
		return nil, "", err
	}
	return out, p.TargetEncoding, nil
}

func encodeStrictBytes(encName string, utf8Bytes []byte) ([]byte, error) {
	if encName == EncodingUTF8 {
		out := make([]byte, len(utf8Bytes))
		copy(out, utf8Bytes)
		return out, nil
	}

	enc, err := lookupEncoding(encName)
	if err != nil {
		return nil, ErrUnsupportedEncoding
	}

	r := transform.NewReader(bytes.NewReader(utf8Bytes), enc.NewEncoder())
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, ErrEncodeFailed
	}

	// 严格失败：防止 encoder 静默替换不可表示字符。
	roundtrip, derr := decodeStrictBytes(encName, out)
	if derr != nil {
		return nil, ErrEncodeFailed
	}
	if !bytes.Equal(roundtrip, utf8Bytes) {
		return nil, ErrUnrepresentable
	}

	return out, nil
}

