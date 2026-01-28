package text

import (
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func TestDetectUTF8(t *testing.T) {
	b := []byte("hello, 世界\n")
	isText, enc := DetectTextAndEncoding(b)
	if !isText || enc != EncodingUTF8 {
		t.Fatalf("expected UTF-8 text, got isText=%v enc=%s", isText, enc)
	}
}

func TestDetectBinaryNUL(t *testing.T) {
	b := []byte{0x00, 0x01, 0x02, 0x03}
	isText, enc := DetectTextAndEncoding(b)
	if isText || enc != EncodingUnknown {
		t.Fatalf("expected non-text, got isText=%v enc=%s", isText, enc)
	}
}

func TestDetectGBKAsGB18030(t *testing.T) {
	src := "中文测试,abc\n"
	gbkBytes, _, err := transform.String(simplifiedchinese.GBK.NewEncoder(), src)
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}

	isText, enc := DetectTextAndEncoding([]byte(gbkBytes))
	if !isText {
		t.Fatalf("expected text")
	}
	// 按设计顺序：GB18030 在 GBK 之前，因此大概率会命中 GB18030。
	if enc != EncodingGB18030 && enc != EncodingGBK {
		t.Fatalf("expected GB18030/GBK, got %s", enc)
	}
}

func TestStrictTranscodeEmojiToGBKFails(t *testing.T) {
	src := []byte("hello🙂")
	_, _, err := StrictTranscode(src, TranscodeParams{SourceEncoding: EncodingUTF8, TargetEncoding: EncodingGBK})
	if err != ErrUnrepresentable && err != ErrEncodeFailed {
		t.Fatalf("expected unrepresentable/encode failed, got %v", err)
	}
}

func TestStrictTranscodeGBKToUTF8(t *testing.T) {
	src := "中文,log-" + time.Unix(1, 0).UTC().Format(time.RFC3339) + "\n"
	gbkBytes, _, err := transform.String(simplifiedchinese.GBK.NewEncoder(), src)
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}

	out, _, err := StrictTranscode([]byte(gbkBytes), TranscodeParams{SourceEncoding: EncodingGBK, TargetEncoding: EncodingUTF8})
	if err != nil {
		t.Fatalf("transcode: %v", err)
	}
	if string(out) != src {
		t.Fatalf("expected %q, got %q", src, string(out))
	}
}

