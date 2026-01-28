package text

import (
	"errors"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const (
	EncodingUnknown     = "Unknown"
	EncodingUTF8        = "UTF-8"
	EncodingGB18030     = "GB18030"
	EncodingGBK         = "GBK"
	EncodingBig5        = "Big5"
	EncodingWindows1252 = "Windows-1252"
	EncodingISO88591    = "ISO-8859-1"

	SourceEncodingAuto = "auto"
)

// TargetEncodings 返回“目标编码下拉框”的最终清单与展示顺序。
func TargetEncodings() []string {
	return []string{
		EncodingUTF8,
		EncodingGB18030,
		EncodingGBK,
		EncodingBig5,
		EncodingWindows1252,
		EncodingISO88591,
	}
}

func lookupEncoding(name string) (encoding.Encoding, error) {
	switch strings.TrimSpace(name) {
	case EncodingUTF8:
		return encoding.Nop, nil
	case EncodingGB18030:
		return simplifiedchinese.GB18030, nil
	case EncodingGBK:
		return simplifiedchinese.GBK, nil
	case EncodingBig5:
		return traditionalchinese.Big5, nil
	case EncodingWindows1252:
		return charmap.Windows1252, nil
	case EncodingISO88591:
		return charmap.ISO8859_1, nil
	default:
		return nil, errors.New("unknown encoding")
	}
}

