package fastxml

import (
	"strings"
	"testing"
)

// --- fixtures ---

const smallXML = `<?xml version="1.0" encoding="UTF-8"?>
<person id="1" role="admin">
  <name>Alice</name>
  <age>30</age>
  <email>alice@example.com</email>
</person>`

const mediumXML = `<?xml version="1.0"?>
<catalog>
  <book id="bk101" lang="en">
    <author>Gambardella, Matthew</author>
    <title>XML Developer's Guide</title>
    <genre>Computer</genre>
    <price>44.95</price>
    <publish_date>2000-10-01</publish_date>
    <description>An in-depth look at creating applications with XML.</description>
  </book>
  <book id="bk102" lang="en">
    <author>Ralls, Kim</author>
    <title>Midnight Rain</title>
    <genre>Fantasy</genre>
    <price>5.95</price>
    <publish_date>2000-12-16</publish_date>
    <description>A former architect battles corporate zombies.</description>
  </book>
  <book id="bk103" lang="fr">
    <author>Corets, Eva</author>
    <title>Maeve Ascendant</title>
    <genre>Fantasy</genre>
    <price>5.95</price>
    <publish_date>2000-11-17</publish_date>
    <description>After the collapse of a nanotechnology society.</description>
  </book>
</catalog>`

const cdataXML = `<root><data><![CDATA[Hello <world> & "friends"]]></data></root>`

const attrXML = `<root><item id="1" name="foo" value="bar"/><item id="2" name="baz" value="qux"/></root>`

const entityXML = `<root><text>AT&amp;T &lt;rocks&gt; &quot;yes&quot;</text></root>`

// generateLargeXML builds a synthetic ~100KB XML document.
func generateLargeXML() string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n<root>\n")
	for i := 0; i < 500; i++ {
		sb.WriteString(`  <record id="`)
		writeInt(&sb, i)
		sb.WriteString(`" category="cat`)
		writeInt(&sb, i%10)
		sb.WriteString(`">`)
		sb.WriteString("\n    <name>Record ")
		writeInt(&sb, i)
		sb.WriteString("</name>\n    <value>")
		writeInt(&sb, i*17)
		sb.WriteString("</value>\n    <description>This is a description for record number ")
		writeInt(&sb, i)
		sb.WriteString(" with some extra content to make it longer.</description>\n  </record>\n")
	}
	sb.WriteString("</root>")
	return sb.String()
}

func writeInt(sb *strings.Builder, n int) {
	if n == 0 {
		sb.WriteByte('0')
		return
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	sb.Write(buf[pos:])
}

// --- unit tests ---

func TestParseSimple(t *testing.T) {
	var p Parser
	v, err := p.Parse(smallXML)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name() != "person" {
		t.Fatalf("expected root name 'person', got %q", v.Name())
	}
	if v.GetAttr("id") != "1" {
		t.Fatalf("expected attr id=1, got %q", v.GetAttr("id"))
	}
	if v.GetAttr("role") != "admin" {
		t.Fatalf("expected attr role=admin")
	}

	name := v.Get("name")
	if name == nil {
		t.Fatal("expected <name> child")
	}
	if name.Text() != "Alice" {
		t.Fatalf("expected text Alice, got %q", name.Text())
	}

	age := v.Get("age")
	if age == nil {
		t.Fatal("expected <age> child")
	}
	if age.Text() != "30" {
		t.Fatalf("expected text 30, got %q", age.Text())
	}
}

func TestParseCDATA(t *testing.T) {
	var p Parser
	v, err := p.Parse(cdataXML)
	if err != nil {
		t.Fatal(err)
	}
	data := v.Get("data")
	if data == nil {
		t.Fatal("expected <data> child")
	}
	text := data.Text()
	if text != `Hello <world> & "friends"` {
		t.Fatalf("unexpected CDATA text: %q", text)
	}
}

func TestParseAttributes(t *testing.T) {
	var p Parser
	v, err := p.Parse(attrXML)
	if err != nil {
		t.Fatal(err)
	}
	items := v.Children("item")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].GetAttr("id") != "1" {
		t.Fatalf("expected id=1, got %q", items[0].GetAttr("id"))
	}
	if items[1].GetAttr("name") != "baz" {
		t.Fatalf("expected name=baz, got %q", items[1].GetAttr("name"))
	}
}

func TestParseEntityEscape(t *testing.T) {
	var p Parser
	v, err := p.Parse(entityXML)
	if err != nil {
		t.Fatal(err)
	}
	text := v.Get("text")
	if text == nil {
		t.Fatal("expected <text> child")
	}
	got := text.Text()
	want := `AT&T <rocks> "yes"`
	if got != want {
		t.Fatalf("entity unescape: got %q want %q", got, want)
	}
}

func TestParseArrayPromotion(t *testing.T) {
	var p Parser
	v, err := p.Parse(mediumXML)
	if err != nil {
		t.Fatal(err)
	}
	books := v.Children("book")
	if len(books) != 3 {
		t.Fatalf("expected 3 books, got %d", len(books))
	}
	if books[0].GetAttr("id") != "bk101" {
		t.Fatalf("expected bk101, got %q", books[0].GetAttr("id"))
	}
	if books[2].GetAttr("lang") != "fr" {
		t.Fatalf("expected fr, got %q", books[2].GetAttr("lang"))
	}
}

func TestMarshalToSimple(t *testing.T) {
	var p Parser
	v, err := p.Parse(`<msg>hello</msg>`)
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.MarshalTo(nil))
	want := `{"msg":"hello"}`
	if got != want {
		t.Fatalf("MarshalTo: got %s want %s", got, want)
	}
}

func TestMarshalToWithAttrs(t *testing.T) {
	var p Parser
	v, err := p.Parse(`<item id="42">content</item>`)
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.MarshalTo(nil))
	// should contain @id and #text
	if !strings.Contains(got, `"@id":"42"`) {
		t.Fatalf("missing @id: %s", got)
	}
	if !strings.Contains(got, `"#text":"content"`) {
		t.Fatalf("missing #text: %s", got)
	}
}

func TestMarshalToArray(t *testing.T) {
	var p Parser
	v, err := p.Parse(`<root><item>a</item><item>b</item><item>c</item></root>`)
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.MarshalTo(nil))
	if !strings.Contains(got, `["a","b","c"]`) {
		t.Fatalf("expected array: %s", got)
	}
}

func TestMarshalToNull(t *testing.T) {
	var p Parser
	v, err := p.Parse(`<root><empty/></root>`)
	if err != nil {
		t.Fatal(err)
	}
	got := string(v.MarshalTo(nil))
	if !strings.Contains(got, `"empty":null`) {
		t.Fatalf("expected null for empty element: %s", got)
	}
}

func TestParserPool(t *testing.T) {
	var pool ParserPool
	p := pool.Get()
	v, err := p.Parse(smallXML)
	if err != nil {
		t.Fatal(err)
	}
	if v.Name() != "person" {
		t.Fatalf("expected person, got %q", v.Name())
	}
	pool.Put(p)

	// get again and parse something else
	p2 := pool.Get()
	v2, err := p2.Parse(`<x>y</x>`)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Text() != "y" {
		t.Fatalf("expected y, got %q", v2.Text())
	}
	pool.Put(p2)
}

func TestSelfClosingTag(t *testing.T) {
	var p Parser
	v, err := p.Parse(`<root><br/><hr/></root>`)
	if err != nil {
		t.Fatal(err)
	}
	br := v.Get("br")
	if br == nil {
		t.Fatal("expected <br/>")
	}
	if br.Text() != "" {
		t.Fatalf("expected empty text for br, got %q", br.Text())
	}
}

func TestDeepNesting(t *testing.T) {
	xml := `<a><b><c><d><e>deep</e></d></c></b></a>`
	var p Parser
	v, err := p.Parse(xml)
	if err != nil {
		t.Fatal(err)
	}
	deep := v.Get("b", "c", "d", "e")
	if deep == nil {
		t.Fatal("deep path not found")
	}
	if deep.Text() != "deep" {
		t.Fatalf("expected deep, got %q", deep.Text())
	}
}

func TestComment(t *testing.T) {
	xml := `<root><!-- this is a comment --><child>value</child></root>`
	var p Parser
	v, err := p.Parse(xml)
	if err != nil {
		t.Fatal(err)
	}
	child := v.Get("child")
	if child == nil || child.Text() != "value" {
		t.Fatalf("expected child with value, got %v", child)
	}
}

// --- benchmarks ---

var benchPool ParserPool
var largXML = generateLargeXML()

func BenchmarkParseSmall(b *testing.B) {
	benchmarkParse(b, smallXML)
}

func BenchmarkParseMedium(b *testing.B) {
	benchmarkParse(b, mediumXML)
}

func BenchmarkParseLarge(b *testing.B) {
	benchmarkParse(b, largXML)
}

func benchmarkParse(b *testing.B, s string) {
	b.Helper()
	b.ReportAllocs()
	b.SetBytes(int64(len(s)))
	b.RunParallel(func(pb *testing.PB) {
		p := benchPool.Get()
		for pb.Next() {
			v, err := p.Parse(s)
			if err != nil {
				b.Fatal(err)
			}
			_ = v
		}
		benchPool.Put(p)
	})
}

func BenchmarkMarshalToSmall(b *testing.B) {
	benchmarkMarshalTo(b, smallXML)
}

func BenchmarkMarshalToMedium(b *testing.B) {
	benchmarkMarshalTo(b, mediumXML)
}

func BenchmarkMarshalToLarge(b *testing.B) {
	benchmarkMarshalTo(b, largXML)
}

func benchmarkMarshalTo(b *testing.B, s string) {
	b.Helper()
	p := benchPool.Get()
	v, err := p.Parse(s)
	if err != nil {
		b.Fatal(err)
	}
	benchPool.Put(p)

	b.ReportAllocs()
	b.SetBytes(int64(len(s)))
	dst := make([]byte, 0, len(s)*2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dst = v.MarshalTo(dst[:0])
	}
	_ = dst
}

func BenchmarkGet(b *testing.B) {
	p := benchPool.Get()
	v, err := p.Parse(mediumXML)
	if err != nil {
		b.Fatal(err)
	}
	benchPool.Put(p)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Get("book")
	}
}
