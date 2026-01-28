package text

import (
	"bytes"
	"io"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/transform"
)

const (
	maxDetectSampleBytes = 64 * 1024

	// 更保守：出现 NUL 直接判二进制。
	maxNULAllowed = 0

	// 更保守：控制字符占比超过阈值则判二进制。
	maxBadControlRatio = 0.01

	// 更保守：可打印字符占比阈值（通用）。
	minPrintableRatio = 0.95

	// 对“任意字节都可解码”的单字节编码更严格，避免误放行二进制。
	minPrintableRatioSingleByte = 0.98
	minTextRunesSingleByte      = 20
)

// DetectTextAndEncoding 以“宁可少放行”的策略判断是否为可识别文本，并给出最可能的编码。
// 注意：该函数主要用于“是否开放转码”的保守判定；转码时仍需对全量 bytes 做严格解码校验。
func DetectTextAndEncoding(b []byte) (isText bool, enc string) {
	sample := b
	if len(sample) > maxDetectSampleBytes {
		sample = sample[:maxDetectSampleBytes]
	}

	if looksBinary(sample) {
		return false, EncodingUnknown
	}

	if utf8.Valid(sample) && printableRatioUTF8(sample) >= minPrintableRatio {
		return true, EncodingUTF8
	}

	// 非 UTF-8：按候选列表顺序尝试严格解码（偏保守）。
	if ok := tryEncoding(sample, EncodingGB18030, minPrintableRatio, 0); ok {
		return true, EncodingGB18030
	}
	if ok := tryEncoding(sample, EncodingGBK, minPrintableRatio, 0); ok {
		return true, EncodingGBK
	}
	if ok := tryEncoding(sample, EncodingBig5, minPrintableRatio, 0); ok {
		return true, EncodingBig5
	}
	if ok := tryEncoding(sample, EncodingWindows1252, minPrintableRatioSingleByte, minTextRunesSingleByte); ok {
		return true, EncodingWindows1252
	}
	if ok := tryEncoding(sample, EncodingISO88591, minPrintableRatioSingleByte, minTextRunesSingleByte); ok {
		return true, EncodingISO88591
	}

	return false, EncodingUnknown
}

func looksBinary(sample []byte) bool {
	if len(sample) == 0 {
		return true
	}

	var nul int
	var badCtrl int

	for _, c := range sample {
		if c == 0x00 {
			nul++
			if nul > maxNULAllowed {
				return true
			}
			continue
		}

		// 允许 \t \n \r
		if c == '\t' || c == '\n' || c == '\r' {
			continue
		}

		// 其他控制字符（含 DEL）计入 bad control
		if c < 0x20 || c == 0x7F {
			badCtrl++
		}
	}

	if len(sample) == 0 {
		return true
	}
	if float64(badCtrl)/float64(len(sample)) > maxBadControlRatio {
		return true
	}
	return false
}

func tryEncoding(sample []byte, encName string, minPrintable float64, minRunes int) bool {
	decoded, err := decodeStrictBytes(encName, sample)
	if err != nil {
		return false
	}
	if bytes.ContainsRune(decoded, unicode.ReplacementChar) {
		return false
	}

	ratio, runes := printableRatioRunes(string(decoded))
	if runes == 0 {
		return false
	}
	if ratio < minPrintable {
		return false
	}
	if minRunes > 0 && runes < minRunes {
		return false
	}
	return true
}

func printableRatioUTF8(sample []byte) float64 {
	return printableRatioRunes(string(sample))
}

func printableRatioRunes(s string) (ratio float64, runes int) {
	var printable int
	for _, r := range s {
		runes++
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			printable++
		case unicode.IsPrint(r):
			printable++
		}
	}
	if runes == 0 {
		return 0, 0
	}
	return float64(printable) / float64(runes), runes
}

func decodeStrictBytes(encName string, src []byte) ([]byte, error) {
	if encName == EncodingUTF8 {
		if !utf8.Valid(src) {
			return nil, ErrDecodeFailed
		}
		out := make([]byte, len(src))
		copy(out, src)
		return out, nil
	}

	enc, err := lookupEncoding(encName)
	if err != nil {
		return nil, ErrUnsupportedEncoding
	}

	r := transform.NewReader(bytes.NewReader(src), enc.NewDecoder())
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, ErrDecodeFailed
	}
	return out, nil
}

