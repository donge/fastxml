package fastxml

import (
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// encodeGBK converts a UTF-8 string to GBK bytes for test fixture generation.
func encodeGBK(s string) []byte {
	b, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
	if err != nil {
		panic(err)
	}
	return b
}

func encodeBig5(s string) []byte {
	b, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(s))
	if err != nil {
		panic(err)
	}
	return b
}

// --- unit tests ---

func TestParseEncoded_UTF8PassThrough(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?><root><name>测试</name></root>`
	var p Parser
	v, err := p.ParseEncoded(xml)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("name").Text(); got != "测试" {
		t.Fatalf("got %q, want 测试", got)
	}
}

func TestParseEncoded_NoDecl(t *testing.T) {
	xml := `<root><val>hello</val></root>`
	var p Parser
	v, err := p.ParseEncoded(xml)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.Get("val").Text(); got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestParseEncoded_GBK(t *testing.T) {
	// Build GBK-encoded XML: <?xml version="1.0" encoding="GBK"?><root><城市>北京</城市></root>
	utf8body := `<?xml version="1.0" encoding="GBK"?><root><城市>北京</城市></root>`
	gbkBytes := encodeGBK(utf8body)

	var p Parser
	v, err := p.ParseBytesEncoded(gbkBytes)
	if err != nil {
		t.Fatalf("ParseBytesEncoded GBK: %v", err)
	}
	city := v.Get("城市")
	if city == nil {
		t.Fatal("expected <城市> child")
	}
	if got := city.Text(); got != "北京" {
		t.Fatalf("got %q, want 北京", got)
	}
}

func TestParseEncoded_GB18030(t *testing.T) {
	utf8body := `<?xml version="1.0" encoding="GB18030"?><data><item>中文内容</item></data>`
	enc, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte(utf8body))
	if err != nil {
		t.Fatal(err)
	}
	var p Parser
	v, err := p.ParseBytesEncoded(enc)
	if err != nil {
		t.Fatalf("ParseBytesEncoded GB18030: %v", err)
	}
	if got := v.Get("item").Text(); got != "中文内容" {
		t.Fatalf("got %q, want 中文内容", got)
	}
}

func TestParseEncoded_Big5(t *testing.T) {
	// Big5 fixture — Traditional Chinese: 台北, 繁體中文
	utf8body := `<?xml version="1.0" encoding="Big5"?><root><city>台北</city></root>`
	big5Bytes := encodeBig5(utf8body)

	var p Parser
	v, err := p.ParseBytesEncoded(big5Bytes)
	if err != nil {
		t.Fatalf("ParseBytesEncoded Big5: %v", err)
	}
	if got := v.Get("city").Text(); got != "台北" {
		t.Fatalf("got %q, want 台北", got)
	}
}

func TestParseEncoded_ISO8859_1(t *testing.T) {
	// ISO-8859-1: é = 0xE9
	iso := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><root><val>caf\xe9</val></root>")
	var p Parser
	v, err := p.ParseBytesEncoded(iso)
	if err != nil {
		t.Fatalf("ParseBytesEncoded ISO-8859-1: %v", err)
	}
	if got := v.Get("val").Text(); got != "café" {
		t.Fatalf("got %q, want café", got)
	}
}

func TestParseEncoded_UTF8BOM(t *testing.T) {
	// BOM + UTF-8 content, no encoding declaration
	xml := "\xef\xbb\xbf<root><x>bom</x></root>"
	var p Parser
	v, err := p.ParseEncoded(xml)
	if err != nil {
		t.Fatalf("BOM UTF-8: %v", err)
	}
	if got := v.Get("x").Text(); got != "bom" {
		t.Fatalf("got %q, want bom", got)
	}
}

func TestParseEncoded_UnsupportedEncoding(t *testing.T) {
	xml := `<?xml version="1.0" encoding="SHIFT_JIS"?><root/>`
	var p Parser
	_, err := p.ParseEncoded(xml)
	if err == nil {
		t.Fatal("expected error for unsupported encoding")
	}
}

// --- benchmarks ---

var (
	gbkLargeXML  []byte
	utf8LargeXML string
)

func init() {
	// Build a ~50KB GBK XML with Chinese content.
	const head = `<?xml version="1.0" encoding="UTF-8"?><root>`
	const tail = `</root>`
	items := make([]byte, 0, 60000)
	items = append(items, head...)
	for i := 0; i < 300; i++ {
		items = append(items, "<记录><名称>测试记录"...)
		items = append(items, "<编号>12345</编号>"...)
		items = append(items, "<描述>这是一段用于测试的中文描述内容，包含常用汉字。</描述>"...)
		items = append(items, "</名称></记录>"...)
	}
	items = append(items, tail...)

	var err error
	gbkLargeXML, err = simplifiedchinese.GBK.NewEncoder().Bytes(items)
	if err != nil {
		panic(err)
	}
	utf8LargeXML = string(items) // same content, UTF-8
}

// BenchmarkParseEncoded_UTF8 — ParseEncoded on UTF-8 input.
// Should be within a few ns of BenchmarkParseLarge (just header scan overhead).
func BenchmarkParseEncoded_UTF8(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(utf8LargeXML)))
	b.RunParallel(func(pb *testing.PB) {
		p := benchPool.Get()
		for pb.Next() {
			v, err := p.ParseEncoded(utf8LargeXML)
			if err != nil {
				b.Fatal(err)
			}
			_ = v
		}
		benchPool.Put(p)
	})
}

// BenchmarkParse_UTF8Baseline — direct Parse on the same UTF-8 input.
// Use this as the reference line against BenchmarkParseEncoded_UTF8.
func BenchmarkParse_UTF8Baseline(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(utf8LargeXML)))
	b.RunParallel(func(pb *testing.PB) {
		p := benchPool.Get()
		for pb.Next() {
			v, err := p.Parse(utf8LargeXML)
			if err != nil {
				b.Fatal(err)
			}
			_ = v
		}
		benchPool.Put(p)
	})
}

// BenchmarkParseEncoded_GBK — ParseEncoded on GBK input (conversion path).
func BenchmarkParseEncoded_GBK(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(gbkLargeXML)))
	b.RunParallel(func(pb *testing.PB) {
		p := benchPool.Get()
		for pb.Next() {
			v, err := p.ParseBytesEncoded(gbkLargeXML)
			if err != nil {
				b.Fatal(err)
			}
			_ = v
		}
		benchPool.Put(p)
	})
}
