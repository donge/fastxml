package fastxml

import "sync"

// ParserPool is a pool of Parsers for concurrent use.
// Get a Parser, use it, then Put it back.
type ParserPool struct {
	pool sync.Pool
}

// Get returns a Parser from the pool.
func (pp *ParserPool) Get() *Parser {
	v := pp.pool.Get()
	if v == nil {
		return &Parser{}
	}
	return v.(*Parser)
}

// Put returns p to the pool.
func (pp *ParserPool) Put(p *Parser) {
	pp.pool.Put(p)
}

// Arena supports building Value trees programmatically (e.g. for testing).
// Not safe for concurrent use. Call Reset between uses.
type Arena struct {
	c cache
	b []byte // backing store for programmatically created strings
}

// Reset resets the Arena for reuse. All Values obtained from this Arena
// become invalid after Reset.
func (a *Arena) Reset() {
	a.c.reset()
	a.b = a.b[:0]
}

// NewElement creates a new element Value with the given tag name.
func (a *Arena) NewElement(name string) *Value {
	v := a.c.getValue()
	v.t = TypeElement
	start := len(a.b)
	a.b = append(a.b, name...)
	v.name = b2s(a.b[start:])
	return v
}

// NewText creates a new text Value.
func (a *Arena) NewText(text string) *Value {
	v := a.c.getValue()
	v.t = TypeText
	start := len(a.b)
	a.b = append(a.b, text...)
	v.text = b2s(a.b[start:])
	return v
}

// AddChild appends child to parent's children list.
func (a *Arena) AddChild(parent, child *Value) {
	appendChildWithArrayPromotion(&a.c, parent, child)
}

// SetAttr sets an attribute on v.
func (a *Arena) SetAttr(v *Value, name, value string) {
	for i := range v.attrs {
		if v.attrs[i].name == name {
			start := len(a.b)
			a.b = append(a.b, value...)
			v.attrs[i].value = b2s(a.b[start:])
			return
		}
	}
	at := a.c.getAttr()
	ns := len(a.b)
	a.b = append(a.b, name...)
	at.name = b2s(a.b[ns:])
	vs := len(a.b)
	a.b = append(a.b, value...)
	at.value = b2s(a.b[vs:])
	v.attrs = append(v.attrs, *at)
}
