package fastxml

import (
	"fmt"
	"strings"
)

func skipBOM(s string) string {
	if len(s) >= 3 && s[0] == 0xEF && s[1] == 0xBB && s[2] == 0xBF {
		return s[3:]
	}
	return s
}

func skipWhitespace(s string) string {
	for len(s) > 0 && isSpace(s[0]) {
		s = s[1:]
	}
	return s
}

// skipXMLDecl advances past <?xml ...?> if present.
func skipXMLDecl(s string) string {
	if len(s) >= 5 && s[:5] == "<?xml" {
		i := strings.Index(s, "?>")
		if i >= 0 {
			return s[i+2:]
		}
	}
	return s
}

// skipComment advances past <!-- ... -->, called after consuming "<!--".
func skipComment(s string) (string, error) {
	i := strings.Index(s, "-->")
	if i < 0 {
		return s, fmt.Errorf("fastxml: unterminated comment")
	}
	return s[i+3:], nil
}

// skipPI advances past <? ... ?>, called after consuming "<?".
func skipPI(s string) (string, error) {
	i := strings.Index(s, "?>")
	if i < 0 {
		return s, fmt.Errorf("fastxml: unterminated processing instruction")
	}
	return s[i+2:], nil
}

// parseCDATA parses a CDATA section, called after consuming "<![CDATA[".
func (c *cache) parseCDATA(s string) (*Value, string, error) {
	i := strings.Index(s, "]]>")
	if i < 0 {
		return nil, s, fmt.Errorf("fastxml: unterminated CDATA section")
	}
	v := c.getValue()
	v.t = TypeCDATA
	v.text = s[:i]
	return v, s[i+3:], nil
}

// parseText parses a text node up to the next '<'.
func (c *cache) parseText(s string) (*Value, string) {
	i := strings.IndexByte(s, '<')
	if i < 0 {
		i = len(s)
	}
	v := c.getValue()
	v.t = TypeText
	v.text = s[:i]
	return v, s[i:]
}

// scanName returns the next name token (zero-copy sub-slice).
func scanName(s string) (name, rest string, err error) {
	i := 0
	for i < len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' ||
			c == '>' || c == '/' || c == '=' {
			break
		}
		i++
	}
	if i == 0 {
		return "", s, fmt.Errorf("fastxml: expected name, got %.10q", s)
	}
	return s[:i], s[i:], nil
}

// scanQuotedString returns the content between ' or " quotes (zero-copy sub-slice).
func scanQuotedString(s string) (val, rest string, err error) {
	if len(s) == 0 {
		return "", s, fmt.Errorf("fastxml: expected quoted string, got EOF")
	}
	q := s[0]
	if q != '\'' && q != '"' {
		return "", s, fmt.Errorf("fastxml: expected quote, got %q", q)
	}
	s = s[1:]
	i := strings.IndexByte(s, q)
	if i < 0 {
		return "", s, fmt.Errorf("fastxml: unterminated attribute value")
	}
	return s[:i], s[i+1:], nil
}

// parseElement parses one XML element (including its children) from s.
func (c *cache) parseElement(s string, depth int) (*Value, string, error) {
	if depth > MaxDepth {
		return nil, s, fmt.Errorf("fastxml: exceeded maximum nesting depth %d", MaxDepth)
	}

	s = skipWhitespace(s)
	if len(s) == 0 {
		return nil, s, fmt.Errorf("fastxml: unexpected EOF")
	}

	if s[0] != '<' {
		// text node
		v, rest := c.parseText(s)
		return v, rest, nil
	}

	if len(s) >= 4 && s[:4] == "<!--" {
		rest, err := skipComment(s[4:])
		if err != nil {
			return nil, s, err
		}
		return c.parseElement(rest, depth)
	}

	if len(s) >= 2 && s[1] == '?' {
		rest, err := skipPI(s[2:])
		if err != nil {
			return nil, s, err
		}
		return c.parseElement(rest, depth)
	}

	if len(s) >= 9 && s[:9] == "<![CDATA[" {
		return c.parseCDATA(s[9:])
	}

	// skip <!DOCTYPE ...> and any other <!...> declarations (not CDATA, not comments)
	if len(s) >= 2 && s[1] == '!' {
		rest, err := skipBangDecl(s[2:])
		if err != nil {
			return nil, s, err
		}
		return c.parseElement(rest, depth)
	}

	// open tag
	s = s[1:] // consume '<'
	tagName, s, err := scanName(s)
	if err != nil {
		return nil, s, fmt.Errorf("fastxml: invalid tag name: %w", err)
	}

	v := c.getValue()
	v.t = TypeElement
	v.name = tagName

	// save the start of attrs in the slab so we can take a sub-slice
	attrStart := len(c.as)

	// parse attributes
	for {
		s = skipWhitespace(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("fastxml: unexpected EOF in tag <%s>", tagName)
		}
		if s[0] == '/' {
			// self-closing
			if len(s) < 2 || s[1] != '>' {
				return nil, s, fmt.Errorf("fastxml: expected '/>' in tag <%s>", tagName)
			}
			s = s[2:]
			v.attrs = c.as[attrStart:]
			return v, s, nil
		}
		if s[0] == '>' {
			s = s[1:]
			break
		}
		// attribute
		a := c.getAttr()
		a.name, s, err = scanName(s)
		if err != nil {
			return nil, s, fmt.Errorf("fastxml: invalid attribute name in <%s>: %w", tagName, err)
		}
		s = skipWhitespace(s)
		if len(s) == 0 || s[0] != '=' {
			// boolean attribute (no value); treat as name=""
			a.value = ""
			continue
		}
		s = s[1:] // consume '='
		s = skipWhitespace(s)
		a.value, s, err = scanQuotedString(s)
		if err != nil {
			return nil, s, fmt.Errorf("fastxml: invalid attribute value in <%s>: %w", tagName, err)
		}
	}
	v.attrs = c.as[attrStart:]

	// parse children
	childPtrStart := len(c.vs) // not used directly but we manage via v.children
	_ = childPtrStart

	for {
		s = skipWhitespace(s)
		if len(s) == 0 {
			return nil, s, fmt.Errorf("fastxml: unclosed tag <%s>", tagName)
		}

		// close tag
		if len(s) >= 2 && s[0] == '<' && s[1] == '/' {
			s = s[2:]
			closeName, rest, err := scanName(s)
			if err != nil {
				return nil, s, fmt.Errorf("fastxml: invalid close tag: %w", err)
			}
			if closeName != tagName {
				return nil, s, fmt.Errorf("fastxml: mismatched tags: open <%s> close </%s>", tagName, closeName)
			}
			rest = skipWhitespace(rest)
			if len(rest) == 0 || rest[0] != '>' {
				return nil, rest, fmt.Errorf("fastxml: expected '>' after </%s>", tagName)
			}
			s = rest[1:]
			break
		}

		// skip comments and PIs inside element
		if len(s) >= 4 && s[:4] == "<!--" {
			s, err = skipComment(s[4:])
			if err != nil {
				return nil, s, err
			}
			continue
		}
		if len(s) >= 2 && s[1] == '?' {
			s, err = skipPI(s[2:])
			if err != nil {
				return nil, s, err
			}
			continue
		}

		ch, rest, err := c.parseElement(s, depth+1)
		if err != nil {
			return nil, rest, err
		}
		s = rest

		// skip pure-whitespace text nodes
		if ch.t == TypeText && isAllWhitespace(ch.text) {
			continue
		}

		appendChildWithArrayPromotion(c, v, ch)
	}

	// collapse: single text/CDATA child with no attrs → inline text
	if len(v.attrs) == 0 && len(v.children) == 1 {
		ch := v.children[0]
		if ch.t == TypeText || ch.t == TypeCDATA {
			v.text = ch.text
			if ch.t == TypeCDATA {
				// mark that it was CDATA so Text() skips entity unescaping
				// we reuse the text field; CDATA needs no unescaping
				v.text = ch.text
			}
			v.children = v.children[:0]
		}
	}

	return v, s, nil
}

func isAllWhitespace(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isSpace(s[i]) {
			return false
		}
	}
	return true
}

// appendChildWithArrayPromotion adds ch to parent's children,
// promoting to TypeArray when multiple siblings share the same tag.
func appendChildWithArrayPromotion(c *cache, parent *Value, ch *Value) {
	// Only element nodes participate in array promotion
	if ch.t != TypeElement {
		parent.children = append(parent.children, ch)
		return
	}

	n := len(parent.children)
	if n > 0 {
		prev := parent.children[n-1]
		if prev.name == ch.name {
			if prev.t == TypeArray {
				prev.children = append(prev.children, ch)
				return
			}
			if prev.t == TypeElement {
				// promote prev into a TypeArray
				arr := c.getValue()
				arr.t = TypeArray
				arr.name = prev.name
				arr.children = append(arr.children, prev, ch)
				parent.children[n-1] = arr
				return
			}
		}
	}
	parent.children = append(parent.children, ch)
}

// skipBangDecl skips <!...> declarations such as <!DOCTYPE ...>.
// s is the content after "<!". Handles nested brackets for internal subsets.
func skipBangDecl(s string) (string, error) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				return s[i+1:], nil
			}
		}
	}
	return s, fmt.Errorf("fastxml: unclosed <!declaration>")
}
