package fastxml

import "fmt"

// Type is the XML node type.
type Type int

const (
	TypeElement Type = iota // <tag ...> node
	TypeText                // bare text content
	TypeCDATA               // <![CDATA[...]]>
	TypeArray               // synthetic: wraps repeated sibling elements with the same tag
)

func (t Type) String() string {
	switch t {
	case TypeElement:
		return "element"
	case TypeText:
		return "text"
	case TypeCDATA:
		return "cdata"
	case TypeArray:
		return "array"
	default:
		return "unknown"
	}
}

// attr is a single XML attribute.
// Both fields are sub-slices of Parser.b (zero-copy after parse).
type attr struct {
	name  string
	value string // raw (may contain XML entities); unescaped lazily
}

// Value represents one XML node.
type Value struct {
	name     string   // element tag name (sub-slice of Parser.b)
	attrs    []attr   // attributes
	children []*Value // child nodes
	text     string   // text content (raw; entities present unless rawText is set)
	t        Type
	rawText  bool     // true when text came from CDATA (no entity unescaping needed)
}

func (v *Value) reset() {
	v.name = ""
	v.attrs = v.attrs[:0]
	for i := range v.children {
		v.children[i] = nil
	}
	v.children = v.children[:0]
	v.text = ""
	v.rawText = false
	v.t = TypeElement
}

// Type returns the node type.
func (v *Value) Type() Type {
	return v.t
}

// Name returns the element tag name (empty for text/CDATA nodes).
func (v *Value) Name() string {
	return v.name
}

// Text returns the text content of the node, with XML entities unescaped.
// For TypeElement, returns the concatenated text of direct text children.
func (v *Value) Text() string {
	switch v.t {
	case TypeText:
		return unescapeXMLEntities(v.text)
	case TypeCDATA:
		return v.text
	case TypeElement:
		if len(v.children) == 0 {
			if v.rawText {
				return v.text
			}
			return unescapeXMLEntities(v.text)
		}
		var result []byte
		for _, ch := range v.children {
			if ch.t == TypeText {
				result = append(result, unescapeXMLEntities(ch.text)...)
			} else if ch.t == TypeCDATA {
				result = append(result, ch.text...)
			}
		}
		return b2s(result)
	}
	return ""
}

// GetAttr returns the raw value of attribute name, or "" if not present.
func (v *Value) GetAttr(name string) string {
	for i := range v.attrs {
		if v.attrs[i].name == name {
			return unescapeXMLEntities(v.attrs[i].value)
		}
	}
	return ""
}

// Get descends the element tree by child tag name path.
// Array nodes are transparent: Get on an array returns the first element.
func (v *Value) Get(path ...string) *Value {
	cur := v
	for _, seg := range path {
		if cur == nil {
			return nil
		}
		if cur.t == TypeArray {
			if len(cur.children) == 0 {
				return nil
			}
			cur = cur.children[0]
		}
		if cur.t != TypeElement {
			return nil
		}
		cur = findChild(cur, seg)
	}
	return cur
}

// Children returns direct child elements matching tag.
// If tag is "", all element children are returned.
// For repeated elements, the TypeArray node's children are returned.
func (v *Value) Children(tag string) []*Value {
	if v.t == TypeArray {
		if tag == "" || v.name == tag {
			return v.children
		}
		return nil
	}
	if v.t != TypeElement {
		return nil
	}
	if tag == "" {
		var out []*Value
		for _, ch := range v.children {
			if ch.t == TypeElement || ch.t == TypeArray {
				out = append(out, ch)
			}
		}
		return out
	}
	for _, ch := range v.children {
		if ch.name == tag {
			if ch.t == TypeArray {
				return ch.children
			}
			return []*Value{ch}
		}
	}
	return nil
}

func findChild(v *Value, name string) *Value {
	for _, ch := range v.children {
		if ch.name == name {
			return ch
		}
	}
	return nil
}

// MarshalTo appends the JSON representation of v to dst and returns the result.
// The root element is wrapped: {"tagName": <value>}.
func (v *Value) MarshalTo(dst []byte) []byte {
	switch v.t {
	case TypeText:
		return appendJSONString(dst, unescapeXMLEntities(v.text))
	case TypeCDATA:
		return appendJSONString(dst, v.text)
	case TypeArray:
		dst = append(dst, '[')
		for i, ch := range v.children {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = marshalElementValue(dst, ch)
		}
		dst = append(dst, ']')
		return dst
	case TypeElement:
		dst = append(dst, '{')
		dst = appendJSONString(dst, v.name)
		dst = append(dst, ':')
		dst = marshalElementValue(dst, v)
		dst = append(dst, '}')
		return dst
	}
	return dst
}

// marshalElementValue marshals the value of an element node (without the wrapping root object).
func marshalElementValue(dst []byte, v *Value) []byte {
	if v.t == TypeArray {
		dst = append(dst, '[')
		for i, ch := range v.children {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = marshalElementValue(dst, ch)
		}
		dst = append(dst, ']')
		return dst
	}

	// Simple element: no attrs, no children
	if len(v.attrs) == 0 && len(v.children) == 0 {
		if v.text == "" {
			return append(dst, "null"...)
		}
		if v.rawText {
			return appendJSONString(dst, v.text)
		}
		return appendJSONString(dst, unescapeXMLEntities(v.text))
	}
	if len(v.attrs) == 0 && len(v.children) == 1 {
		ch := v.children[0]
		if ch.t == TypeText {
			return appendJSONString(dst, unescapeXMLEntities(ch.text))
		}
		if ch.t == TypeCDATA {
			return appendJSONString(dst, ch.text)
		}
	}

	// Complex element: emit object
	dst = append(dst, '{')
	first := true

	// Attributes as "@name"
	for i := range v.attrs {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = append(dst, '"', '@')
		dst = append(dst, v.attrs[i].name...)
		dst = append(dst, '"', ':')
		dst = appendJSONString(dst, unescapeXMLEntities(v.attrs[i].value))
	}

	// Text content as "#text"
	textContent := collectText(v)
	if textContent != "" {
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = append(dst, `"#text":`...)
		dst = appendJSONString(dst, textContent)
	}

	// Element children
	for _, ch := range v.children {
		if ch.t == TypeText || ch.t == TypeCDATA {
			continue
		}
		if !first {
			dst = append(dst, ',')
		}
		first = false
		dst = appendJSONString(dst, ch.name)
		dst = append(dst, ':')
		dst = marshalElementValue(dst, ch)
	}

	dst = append(dst, '}')
	return dst
}

func collectText(v *Value) string {
	if len(v.children) == 0 {
		if v.rawText {
			return v.text
		}
		return unescapeXMLEntities(v.text)
	}
	var b []byte
	for _, ch := range v.children {
		if ch.t == TypeText {
			b = append(b, unescapeXMLEntities(ch.text)...)
		} else if ch.t == TypeCDATA {
			b = append(b, ch.text...)
		}
	}
	return b2s(b)
}

// cache is a slab allocator for Value and attr objects.
type cache struct {
	vs []Value
	as []attr
}

func (c *cache) reset() {
	for i := range c.vs {
		c.vs[i].reset()
	}
	c.vs = c.vs[:0]
	for i := range c.as {
		c.as[i] = attr{}
	}
	c.as = c.as[:0]
}

func (c *cache) getValue() *Value {
	if cap(c.vs) > len(c.vs) {
		c.vs = c.vs[:len(c.vs)+1]
	} else {
		c.vs = append(c.vs, Value{})
	}
	return &c.vs[len(c.vs)-1]
}

func (c *cache) getAttr() *attr {
	if cap(c.as) > len(c.as) {
		c.as = c.as[:len(c.as)+1]
	} else {
		c.as = append(c.as, attr{})
	}
	return &c.as[len(c.as)-1]
}

// MaxDepth is the maximum XML nesting depth.
const MaxDepth = 300

// Parser parses XML documents.
// It is not safe for concurrent use. Use ParserPool for concurrent access.
type Parser struct {
	b []byte
	c cache
}

// Parse parses the XML document s and returns the root element.
// The returned Value is valid until the next Parse call.
func (p *Parser) Parse(s string) (*Value, error) {
	p.b = append(p.b[:0], s...)
	p.c.reset()
	src := b2s(p.b)
	src = skipBOM(src)
	src = skipXMLDecl(src)
	src = skipWhitespace(src)

	// skip top-level comments/PIs before root element
	var err error
	for {
		if len(src) == 0 {
			return nil, fmt.Errorf("fastxml: empty document")
		}
		if len(src) >= 4 && src[:4] == "<!--" {
			src, err = skipComment(src[4:])
			if err != nil {
				return nil, err
			}
			src = skipWhitespace(src)
			continue
		}
		if len(src) >= 2 && src[:2] == "<?" {
			src, err = skipPI(src[2:])
			if err != nil {
				return nil, err
			}
			src = skipWhitespace(src)
			continue
		}
		break
	}

	v, tail, err := p.c.parseElement(src, 0)
	if err != nil {
		return nil, err
	}
	tail = skipWhitespace(tail)
	// allow trailing comments/PIs
	for len(tail) > 0 {
		if len(tail) >= 4 && tail[:4] == "<!--" {
			tail, err = skipComment(tail[4:])
			if err != nil {
				return nil, err
			}
			tail = skipWhitespace(tail)
		} else if len(tail) >= 2 && tail[:2] == "<?" {
			tail, err = skipPI(tail[2:])
			if err != nil {
				return nil, err
			}
			tail = skipWhitespace(tail)
		} else {
			return nil, fmt.Errorf("fastxml: unexpected trailing content: %.20q", tail)
		}
	}
	return v, nil
}

// ParseBytes parses the XML document b. See Parse.
func (p *Parser) ParseBytes(b []byte) (*Value, error) {
	return p.Parse(b2s(b))
}
