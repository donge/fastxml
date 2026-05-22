package fastxml

import (
	"fmt"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
)

// ParseEncoded parses an XML document that may use a non-UTF-8 encoding declared
// in its XML declaration (<?xml ... encoding="GBK"?>).
//
// If the encoding is absent or already UTF-8, this is identical to Parse with
// only the cost of scanning the first ~200 bytes of the declaration.
// Non-UTF-8 input is converted to UTF-8 before parsing; the conversion
// allocates a new buffer (unavoidable for multi-byte encodings with different
// byte widths).
func (p *Parser) ParseEncoded(s string) (*Value, error) {
	enc, contentStart := xmlDeclEncoding(s)
	if enc == "" {
		return p.Parse(s[contentStart:])
	}
	converted, err := toUTF8(s[contentStart:], enc)
	if err != nil {
		return nil, err
	}
	return p.Parse(converted)
}

// ParseBytesEncoded is the []byte variant of ParseEncoded.
func (p *Parser) ParseBytesEncoded(b []byte) (*Value, error) {
	return p.ParseEncoded(b2s(b))
}

// xmlDeclEncoding scans the XML declaration in the first 200 bytes of s.
// Returns the encoding label (raw, not uppercased) and the byte offset of
// content start (past the BOM). Returns ("", 0) when UTF-8 or no declaration.
// Zero allocations on all paths.
func xmlDeclEncoding(s string) (enc string, contentStart int) {
	// Strip UTF-8 BOM.
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		contentStart = 3
		s = s[3:]
	}

	// Must start with "<?xml".
	if len(s) < 5 || s[0] != '<' || s[1] != '?' || s[2] != 'x' || s[3] != 'm' || s[4] != 'l' {
		return "", contentStart
	}

	// Scan at most 200 bytes for the closing "?>".
	limit := 200
	if len(s) < limit {
		limit = len(s)
	}
	end := -1
	for i := 5; i < limit-1; i++ {
		if s[i] == '?' && s[i+1] == '>' {
			end = i
			break
		}
	}
	if end < 0 {
		return "", contentStart
	}

	// Find "encoding" keyword inside the declaration.
	decl := s[5:end] // content between "<?xml" and "?>"
	ei := indexASCII(decl, "encoding")
	if ei < 0 {
		return "", contentStart
	}
	rest := decl[ei+8:]

	// Skip whitespace, expect '='.
	rest = skipASCIISpace(rest)
	if len(rest) == 0 || rest[0] != '=' {
		return "", contentStart
	}
	rest = skipASCIISpace(rest[1:])

	// Quoted value.
	if len(rest) < 2 {
		return "", contentStart
	}
	q := rest[0]
	if q != '"' && q != '\'' {
		return "", contentStart
	}
	closeQ := indexByte(rest[1:], q)
	if closeQ < 0 {
		return "", contentStart
	}
	label := rest[1 : closeQ+1]

	if asciiEqualFold(label, "UTF-8") || asciiEqualFold(label, "UTF8") {
		return "", contentStart
	}
	return label, contentStart
}

// toUTF8 converts s from the named encoding to UTF-8.
func toUTF8(s, enc string) (string, error) {
	dec := lookupDecoder(enc)
	if dec == nil {
		return "", fmt.Errorf("fastxml: unsupported encoding %q", enc)
	}
	out, err := dec.String(s)
	if err != nil {
		return "", fmt.Errorf("fastxml: encoding conversion error (%s): %w", enc, err)
	}
	return out, nil
}

// lookupDecoder maps an XML encoding label (case-insensitive) to an x/text
// decoder. Returns nil for unknown labels. Zero allocations via asciiEqualFold.
func lookupDecoder(enc string) interface{ String(string) (string, error) } {
	switch {
	// Simplified Chinese
	case asciiEqualFold(enc, "GBK") ||
		asciiEqualFold(enc, "GB2312") ||
		asciiEqualFold(enc, "CP936") ||
		asciiEqualFold(enc, "MS936") ||
		asciiEqualFold(enc, "WINDOWS-936"):
		return simplifiedchinese.GBK.NewDecoder()
	case asciiEqualFold(enc, "GB18030"):
		return simplifiedchinese.GB18030.NewDecoder()

	// Traditional Chinese
	case asciiEqualFold(enc, "BIG5") ||
		asciiEqualFold(enc, "BIG-5") ||
		asciiEqualFold(enc, "CN-BIG5") ||
		asciiEqualFold(enc, "CP950"):
		return traditionalchinese.Big5.NewDecoder()

	// Latin-1 / Western European
	case asciiEqualFold(enc, "ISO-8859-1") ||
		asciiEqualFold(enc, "LATIN-1") ||
		asciiEqualFold(enc, "LATIN1") ||
		asciiEqualFold(enc, "ISO8859-1") ||
		asciiEqualFold(enc, "CP1252") ||
		asciiEqualFold(enc, "WINDOWS-1252"):
		return charmap.Windows1252.NewDecoder()

	// UTF-16
	case asciiEqualFold(enc, "UTF-16") || asciiEqualFold(enc, "UTF16"):
		return unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
	case asciiEqualFold(enc, "UTF-16BE") || asciiEqualFold(enc, "UTF16BE"):
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM).NewDecoder()
	case asciiEqualFold(enc, "UTF-16LE") || asciiEqualFold(enc, "UTF16LE"):
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder()
	}
	return nil
}

// --- zero-alloc helpers for ASCII scanning ---

func skipASCIISpace(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return s[i:]
		}
	}
	return ""
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// indexASCII returns the index of the first occurrence of needle in s using
// case-sensitive ASCII comparison. Returns -1 if not found.
func indexASCII(s, needle string) int {
	n := len(needle)
	if n == 0 {
		return 0
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == needle {
			return i
		}
	}
	return -1
}

// asciiEqualFold reports whether s and t are equal under ASCII case-folding.
// Zero allocations.
func asciiEqualFold(s, t string) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		cs, ct := s[i], t[i]
		if cs >= 'a' && cs <= 'z' {
			cs -= 0x20
		}
		if ct >= 'a' && ct <= 'z' {
			ct -= 0x20
		}
		if cs != ct {
			return false
		}
	}
	return true
}
