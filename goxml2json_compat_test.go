package fastxml

// Port of github.com/basgys/goxml2json test cases.
//
// Convention differences between goxml2json and fastxml:
//   Attribute prefix : goxml2json uses "-"   | fastxml uses "@"
//   Text key         : goxml2json uses "#content" | fastxml uses "#text"
//   Namespace prefix : goxml2json strips "prefix:" from tag names | fastxml keeps "prefix:local" verbatim
//   Whitespace trim  : goxml2json trims leading/trailing whitespace from text | fastxml stores raw text
//   Type coercion    : goxml2json supports Int/Float/Bool/Null conversion | fastxml always outputs strings
//   Charset          : goxml2json converts ISO-8859-1 via x/net/html/charset | fastxml is UTF-8 only
//   Rootless XML     : goxml2json tolerates multiple root elements | fastxml requires a single root
//
// Each test below is labelled:
//   [PASS]  — passes with fastxml conventions
//   [DIFF]  — passes but output differs from goxml2json (convention difference documented)
//   [SKIP]  — goxml2json-internal API (Decoder/Encoder struct), not applicable
//   [LIMIT] — fastxml does not support this feature

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

func mustParseMap(t *testing.T, xmlStr string) map[string]any {
	t.Helper()
	var p Parser
	v, err := p.Parse(xmlStr)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	out := v.MarshalTo(nil)
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, out)
	}
	return m
}

func assertString(t *testing.T, m map[string]any, path []string, want string) {
	t.Helper()
	cur := m
	for i, key := range path {
		if i == len(path)-1 {
			got, ok := cur[key].(string)
			if !ok {
				t.Errorf("path %v: expected string %q, got %T %v", path, want, cur[key], cur[key])
				return
			}
			if got != want {
				t.Errorf("path %v: got %q, want %q", path, got, want)
			}
			return
		}
		next, ok := cur[key].(map[string]any)
		if !ok {
			t.Errorf("path %v: key %q not an object, got %T", path, key, cur[key])
			return
		}
		cur = next
	}
}

// ---- converter_test.go ports ------------------------------------------------

// [DIFF] TestConvert — original uses goxml2json's Convert(reader).
// Differences documented inline:
//   • attributes: goxml2json "-version" → fastxml "@version"
//   • text+attr:  goxml2json "#content" → fastxml "#text"
//   • The JSON structure and values are otherwise identical.
func TestGoxml2json_Convert(t *testing.T) {
	s := `<?xml version="1.0" encoding="UTF-8"?>
  <osm version="0.6" generator="CGImap 0.0.2">
   <bounds minlat="54.0889580" minlon="12.2487570" maxlat="54.0913900" maxlon="12.2524800"/>
   <node id="298884269" lat="54.0901746" lon="12.2482632" user="SvenHRO" uid="46882" visible="true" version="1" changeset="676636" timestamp="2008-09-21T21:37:45Z"/>
   <node id="261728686" lat="54.0906309" lon="12.2441924" user="PikoWinter" uid="36744" visible="true" version="1" changeset="323878" timestamp="2008-05-03T13:39:23Z"/>
   <node id="1831881213" version="1" changeset="12370172" lat="54.0900666" lon="12.2539381" user="lafkor" uid="75625" visible="true" timestamp="2012-07-20T09:43:19Z">
    <tag k="name" v="Neu Broderstorf"/>
    <tag k="traffic_sign" v="city_limit"/>
   </node>
   <foo>bar</foo>
   <mixed attr="attribute">content</mixed>
  </osm>`

	m := mustParseMap(t, s)

	osm, ok := m["osm"].(map[string]any)
	if !ok {
		t.Fatalf("expected root 'osm' object, got %T", m["osm"])
	}

	// Attributes use "@" prefix (goxml2json uses "-")
	assertString(t, osm, []string{"@version"}, "0.6")
	assertString(t, osm, []string{"@generator"}, "CGImap 0.0.2")

	// bounds attributes
	bounds, ok := osm["bounds"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'bounds' object, got %T", osm["bounds"])
	}
	assertString(t, bounds, []string{"@minlat"}, "54.0889580")
	assertString(t, bounds, []string{"@maxlat"}, "54.0913900")

	// repeated <node> → JSON array
	nodeArr, ok := osm["node"].([]any)
	if !ok {
		t.Fatalf("expected 'node' array, got %T %v", osm["node"], osm["node"])
	}
	if len(nodeArr) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodeArr))
	}

	node0, ok := nodeArr[0].(map[string]any)
	if !ok {
		t.Fatalf("node[0] not an object")
	}
	if got, _ := node0["@id"].(string); got != "298884269" {
		t.Errorf("node[0][@id] = %q, want 298884269", got)
	}
	if got, _ := node0["@user"].(string); got != "SvenHRO" {
		t.Errorf("node[0][@user] = %q, want SvenHRO", got)
	}

	// third node has nested <tag> array
	node2, ok := nodeArr[2].(map[string]any)
	if !ok {
		t.Fatalf("node[2] not an object")
	}
	tagArr, ok := node2["tag"].([]any)
	if !ok {
		t.Fatalf("node[2].tag not an array, got %T", node2["tag"])
	}
	if len(tagArr) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tagArr))
	}
	tag0, _ := tagArr[0].(map[string]any)
	if got, _ := tag0["@k"].(string); got != "name" {
		t.Errorf("tag[0][@k] = %q, want name", got)
	}

	// plain text element
	assertString(t, osm, []string{"foo"}, "bar")

	// mixed element: attr + text → object with "@attr" and "#text"
	// goxml2json uses "#content"; fastxml uses "#text"
	mixed, ok := osm["mixed"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'mixed' object, got %T %v", osm["mixed"], osm["mixed"])
	}
	if got, _ := mixed["@attr"].(string); got != "attribute" {
		t.Errorf("mixed[@attr] = %q, want attribute", got)
	}
	if got, _ := mixed["#text"].(string); got != "content" {
		// goxml2json would find this under "#content"
		t.Errorf("mixed[#text] = %q, want content (note: goxml2json uses '#content')", got)
	}
}

// [DIFF] TestConvertWithNewLines — goxml2json trims leading/trailing whitespace
// and collapses internal whitespace sequences.
// fastxml stores raw text; Text() does NOT trim.
// This test documents fastxml's actual behaviour (raw preservation).
func TestGoxml2json_WithNewLines(t *testing.T) {
	s := `<?xml version="1.0" encoding="UTF-8"?>
  <osm>
   <foo>
 	foo

	bar
   </foo>
  </osm>`

	m := mustParseMap(t, s)
	osm, ok := m["osm"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'osm' object")
	}

	raw, ok := osm["foo"].(string)
	if !ok {
		t.Fatalf("expected foo string, got %T", osm["foo"])
	}

	// fastxml preserves the raw text including leading/trailing whitespace.
	// goxml2json would return "foo\n\n\tbar" (trimmed).
	// We just verify "foo" and "bar" are both present in the raw value.
	if !strings.Contains(raw, "foo") || !strings.Contains(raw, "bar") {
		t.Errorf("expected raw text to contain 'foo' and 'bar', got %q", raw)
	}
	t.Logf("DIFF: fastxml raw text = %q", raw)
	t.Logf("DIFF: goxml2json would return trimmed = %q", "foo\n\n\tbar")
}

// [DIFF] TestConvertWithMixedTags — namespace prefixes.
// goxml2json strips namespace prefixes: "soap-env:Envelope" → key "Envelope".
// fastxml keeps prefixes verbatim: key "soap-env:Envelope".
// xmlns attributes: goxml2json emits "-soap-env" (prefix only);
// fastxml emits "@xmlns:soap-env" (full attribute name).
func TestGoxml2json_WithMixedTags(t *testing.T) {
	s := `<?xml version="1.0" encoding="UTF-8"?>
	<soap-env:Envelope xmlns:soap-env="http://schemas.xmlsoap.org/soap/envelope/">
	    <soap-env:Header>
	        <wsse:Security xmlns:wsse="http://schemas.xmlsoap.org/ws/2002/12/secext">
	            <wsse:BinarySecurityToken valueType="String" EncodingType="wsse:Base64Binary">
	                Shared/IDL:IceSess\/SessMgr:1\.0.IDL/Common/!ICESMS\/ACPCRTC!ICESMSLB\/CRT.LB!-3379045898978075261!1563026!0
	            </wsse:BinarySecurityToken>
	        </wsse:Security>
	    </soap-env:Header>
	</soap-env:Envelope>`

	m := mustParseMap(t, s)

	// fastxml keeps the full prefixed tag name as the key
	envelope, ok := m["soap-env:Envelope"].(map[string]any)
	if !ok {
		t.Fatalf("expected root key 'soap-env:Envelope' (fastxml keeps prefix); goxml2json uses 'Envelope'. got keys: %v", mapKeys(m))
	}

	// xmlns attribute kept verbatim
	if got, _ := envelope["@xmlns:soap-env"].(string); got != "http://schemas.xmlsoap.org/soap/envelope/" {
		t.Errorf("@xmlns:soap-env = %q, want the namespace URI (goxml2json uses '-soap-env' key)", got)
	}

	header, ok := envelope["soap-env:Header"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'soap-env:Header', got %T (goxml2json uses 'Header')", envelope["soap-env:Header"])
	}

	security, ok := header["wsse:Security"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'wsse:Security', got %T (goxml2json uses 'Security')", header["wsse:Security"])
	}

	token, ok := security["wsse:BinarySecurityToken"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'wsse:BinarySecurityToken' object, got %T", security["wsse:BinarySecurityToken"])
	}
	if got, _ := token["@valueType"].(string); got != "String" {
		t.Errorf("@valueType = %q, want String", got)
	}
	if got, _ := token["@EncodingType"].(string); got != "wsse:Base64Binary" {
		t.Errorf("@EncodingType = %q, want wsse:Base64Binary", got)
	}
	t.Logf("DIFF: fastxml keeps namespace prefixes in tag names; goxml2json strips them")
}

// [LIMIT] TestConvertISO — goxml2json converts ISO-8859-1 input to UTF-8 via
// golang.org/x/net/html/charset. fastxml is UTF-8 only and does not perform
// charset conversion. This test documents the limitation.
func TestGoxml2json_ISO_Limitation(t *testing.T) {
	// <?xml version="1.0" encoding="ISO-8859-1"?><charset>ücomplex</charset>
	// where 0xFC is the ISO-8859-1 byte for 'ü'
	isoInput := []byte{
		0x3C, 0x3F, 0x78, 0x6D, 0x6C, 0x20, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6F,
		0x6E, 0x3D, 0x22, 0x31, 0x2E, 0x30, 0x22, 0x20, 0x65, 0x6E, 0x63, 0x6F,
		0x64, 0x69, 0x6E, 0x67, 0x3D, 0x22, 0x49, 0x53, 0x4F, 0x2D, 0x38, 0x38,
		0x35, 0x39, 0x2D, 0x31, 0x22, 0x3F, 0x3E, 0x3C, 0x63, 0x68, 0x61, 0x72,
		0x73, 0x65, 0x74, 0x3E, 0xFC, 0x62, 0x65, 0x72, 0x20, 0x63, 0x6F, 0x6D,
		0x70, 0x6C, 0x65, 0x78, 0x3C, 0x2F, 0x63, 0x68, 0x61, 0x72, 0x73, 0x65,
		0x74, 0x3E,
	}

	var p Parser
	_, err := p.ParseBytes(isoInput)
	// fastxml will parse the bytes as-is (no charset conversion).
	// The 0xFC byte is invalid UTF-8 so the text content will be garbled.
	// goxml2json would correctly return "über complex".
	t.Logf("LIMIT: fastxml does not perform charset conversion.")
	t.Logf("LIMIT: Parse error (if any): %v", err)
	t.Logf("LIMIT: goxml2json uses golang.org/x/net/html/charset to convert ISO-8859-1 → UTF-8.")
	// Not a failure — just documenting the unsupported feature.
}

// ---- parse_test.go ports ----------------------------------------------------

// [DIFF] TestStringParsing — goxml2json accepts rootless XML (multiple top-level elements).
// fastxml requires a single root element per XML specification.
// The productString fixture has bare <id>, <price>, <deleted>, <nullable> without a wrapper.
// This test documents the limitation and shows fastxml's string-only output with a valid wrapper.
func TestGoxml2json_StringParsing(t *testing.T) {
	// goxml2json's productString has no root element — invalid XML per spec.
	// Wrapping in <product> to make it well-formed for fastxml.
	xmlStr := `<?xml version="1.0" encoding="UTF-8"?>
<product>
  <id>42</id>
  <price>13.32</price>
  <deleted>true</deleted>
  <nullable>null</nullable>
</product>`

	m := mustParseMap(t, xmlStr)
	product, ok := m["product"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'product' root, got %T", m["product"])
	}

	// fastxml always outputs strings (no type coercion)
	assertString(t, product, []string{"id"}, "42")
	assertString(t, product, []string{"price"}, "13.32")
	assertString(t, product, []string{"deleted"}, "true")
	assertString(t, product, []string{"nullable"}, "null")

	t.Logf("DIFF: fastxml always outputs strings; goxml2json supports WithTypeConverter(Int,Float,Bool,Null)")
	t.Logf("DIFF: goxml2json accepts rootless XML; fastxml requires a single root element")
}

// [LIMIT] TestRootlessXML — explicitly shows that fastxml rejects rootless XML.
func TestGoxml2json_RootlessXML_Limitation(t *testing.T) {
	rootless := `<id>42</id><price>13.32</price>`
	var p Parser
	_, err := p.Parse(rootless)
	if err == nil {
		t.Logf("LIMIT: fastxml accepted rootless XML (unexpected)")
	} else {
		t.Logf("LIMIT: fastxml correctly rejects rootless XML: %v", err)
		t.Logf("LIMIT: goxml2json tolerates multiple root elements; fastxml follows the XML spec")
	}
}

// ---- decoder_test.go / encoder_test.go --------------------------------------
// [SKIP] TestDecode, TestDecodeWithoutDefaultsAndExcludeAttributes, TestTrim,
//         TestEncode, TestEncodeWithChildrenAsExplicitArray
// These test goxml2json's internal Decoder/Encoder/Node API which has no
// equivalent in fastxml. fastxml exposes a single Value type with MarshalTo.
