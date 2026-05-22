package fastxml

import (
	"strconv"
	"strings"
	"unsafe"
)

func b2s(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

func s2b(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// nameStop[c] is true for bytes that terminate an XML name token:
// whitespace, '>', '/', '='.
var nameStop [256]bool

// spaceStop[c] is true for bytes that are NOT XML whitespace (used to skip whitespace).
var spaceStop [256]bool

func init() {
	for _, c := range []byte(" \t\n\r>/=") {
		nameStop[c] = true
	}
	for i := 0; i < 256; i++ {
		spaceStop[i] = !(i == ' ' || i == '\t' || i == '\n' || i == '\r')
	}
}

// appendJSONString appends a JSON-encoded string (with quotes) to dst.
// Fast path: if s contains no characters that need escaping, copy directly.
func appendJSONString(dst []byte, s string) []byte {
	if needsEscape(s) {
		return strconv.AppendQuote(dst, s)
	}
	dst = append(dst, '"')
	dst = append(dst, s...)
	dst = append(dst, '"')
	return dst
}

func needsEscape(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c < 0x20 {
			return true
		}
	}
	return false
}

// unescapeXMLEntities returns s with XML entities replaced.
// If s contains no '&', it is returned as-is (zero-alloc fast path).
func unescapeXMLEntities(s string) string {
	if strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b []byte
	for len(s) > 0 {
		i := strings.IndexByte(s, '&')
		if i < 0 {
			b = append(b, s...)
			break
		}
		b = append(b, s[:i]...)
		s = s[i:]
		j := strings.IndexByte(s, ';')
		if j < 0 {
			b = append(b, s...)
			break
		}
		ref := s[1:j]
		switch ref {
		case "amp":
			b = append(b, '&')
		case "lt":
			b = append(b, '<')
		case "gt":
			b = append(b, '>')
		case "apos":
			b = append(b, '\'')
		case "quot":
			b = append(b, '"')
		default:
			if len(ref) > 1 && ref[0] == '#' {
				var n uint64
				var err error
				if ref[1] == 'x' || ref[1] == 'X' {
					n, err = strconv.ParseUint(ref[2:], 16, 32)
				} else {
					n, err = strconv.ParseUint(ref[1:], 10, 32)
				}
				if err == nil {
					b = appendUTF8Rune(b, rune(n))
				} else {
					b = append(b, s[:j+1]...)
				}
			} else {
				b = append(b, s[:j+1]...)
			}
		}
		s = s[j+1:]
	}
	return b2s(b)
}

func appendUTF8Rune(b []byte, r rune) []byte {
	var buf [4]byte
	n := encodeUTF8(buf[:], r)
	return append(b, buf[:n]...)
}

func encodeUTF8(p []byte, r rune) int {
	switch {
	case r < 0x80:
		p[0] = byte(r)
		return 1
	case r < 0x800:
		p[0] = byte(0xC0 | (r >> 6))
		p[1] = byte(0x80 | (r & 0x3F))
		return 2
	case r < 0x10000:
		p[0] = byte(0xE0 | (r >> 12))
		p[1] = byte(0x80 | ((r >> 6) & 0x3F))
		p[2] = byte(0x80 | (r & 0x3F))
		return 3
	default:
		p[0] = byte(0xF0 | (r >> 18))
		p[1] = byte(0x80 | ((r >> 12) & 0x3F))
		p[2] = byte(0x80 | ((r >> 6) & 0x3F))
		p[3] = byte(0x80 | (r & 0x3F))
		return 4
	}
}

// localName strips the namespace prefix ("prefix:local" → "local").
// Zero-alloc: returns a sub-slice of the input.
func localName(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

// trimSpace trims leading and trailing XML whitespace from s.
// Fast path: if first and last bytes are non-space, return s unchanged (zero alloc, zero scan).
func trimSpace(s string) string {
	if len(s) == 0 {
		return s
	}
	if !isSpace(s[0]) && !isSpace(s[len(s)-1]) {
		return s
	}
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}
