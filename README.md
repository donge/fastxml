# fastxml

A high-performance Go XML parser and JSON serializer, inspired by [valyala/fastjson](https://github.com/valyala/fastjson).

## Benchmark Results (Apple M4)

| Operation | Throughput | Allocations |
|---|---|---|
| Parse small (149 B) | **4,365 MB/s** | 0 allocs/op |
| Parse medium (903 B) | **5,293 MB/s** | 0 allocs/op |
| Parse large (107 KB) | **7,303 MB/s** | 0 allocs/op |
| MarshalTo JSON (small) | **2,217 MB/s** | 0 allocs/op |
| MarshalTo JSON (large) | **2,002 MB/s** | 0 allocs/op |
| `Get()` path lookup | **2.7 ns/op** | 0 allocs/op |

Zero allocations in the hot path after warmup.

## Quick Start

```go
import "github.com/donge/fastxml"

// Parse XML
var p fastxml.Parser
v, err := p.Parse(`<person id="1"><name>Alice</name><age>30</age></person>`)
if err != nil {
    log.Fatal(err)
}

// Navigate
fmt.Println(v.Name())              // "person"
fmt.Println(v.GetAttr("id"))       // "1"
fmt.Println(v.Get("name").Text())  // "Alice"

// Serialize to JSON
json := v.MarshalTo(nil)
fmt.Println(string(json))
// {"person":{"@id":"1","name":"Alice","age":"30"}}

// Concurrent use — one pool for the whole program
var pool fastxml.ParserPool

func handle(xmlStr string) {
    p := pool.Get()
    defer pool.Put(p)
    v, _ := p.Parse(xmlStr)
    _ = v.MarshalTo(nil)
}
```

## Features

- **Zero-alloc parsing** in steady state (after slab warmup)
- **XML → JSON** serialization with a single `MarshalTo(dst []byte)` call
- **Navigation**: `Get(path...)`, `GetAttr(name)`, `Children(tag)`, `Text()`
- **CDATA** sections
- **XML entities** (`&amp;`, `&lt;`, `&gt;`, `&apos;`, `&quot;`, `&#nnnn;`) — lazily unescaped
- **Repeated elements** → JSON arrays (automatic promotion)
- **Namespaces** — prefix stored verbatim as `"prefix:local"`, no resolution table
- **Concurrency** via `ParserPool` (wraps `sync.Pool`)
- **Comments and processing instructions** skipped transparently

## XML → JSON Mapping

| XML shape | JSON output |
|---|---|
| `<tag>text</tag>` | `{"tag":"text"}` |
| `<tag/>` (empty) | `{"tag":null}` |
| `<tag id="1">text</tag>` | `{"tag":{"@id":"1","#text":"text"}}` |
| `<root><a/><b/></root>` | `{"root":{"a":null,"b":null}}` |
| `<root><item>a</item><item>b</item></root>` | `{"root":{"item":["a","b"]}}` |
| `<![CDATA[raw]]>` | `"raw"` (no entity escaping) |

**Key conventions:**
- Attributes → `"@attrName"` keys
- Text content alongside other nodes → `"#text"` key
- Repeated sibling elements → JSON array automatically
- Root element is always wrapped: `{"rootTag": ...}`

## Design Principles

fastxml achieves zero allocations and high throughput by applying the same techniques as fastjson to the XML domain.

### 1. Single Input Copy

```
Parse("...xml..."):
  p.b = append(p.b[:0], s...)   ← one allocation, buffer reused across calls
```

Every `Value.name`, `Value.text`, and `attr.value` field is a **sub-slice** of `p.b`.
No string copies happen during parsing. The parser owns the buffer and all derived strings
share the same backing array.

### 2. Slab Allocators

```go
type cache struct {
    vs []Value  // Value slab
    as []attr   // attr slab
}

func (c *cache) reset() {
    c.vs = c.vs[:0]   // length → 0, capacity retained
    c.as = c.as[:0]
}
```

All `Value` and `attr` objects live in two contiguous slices. `reset()` reuses the
backing arrays without releasing them to the GC. After a few parse calls the slabs
reach steady state and no further heap allocations occur.

### 3. Lazy Entity Unescaping

```go
func (v *Value) Text() string {
    if strings.IndexByte(v.text, '&') < 0 {
        return v.text   // fast path: no entities, zero-alloc
    }
    return unescapeXMLEntities(v.text)
}
```

Raw XML text (including `&amp;` etc.) is stored as-is during parsing.
`Text()` first scans for `&` using `strings.IndexByte` (which compiles to an AVX2
`memchr` on amd64/arm64). If there are no entities — the common case — the original
sub-slice is returned with zero allocation.

### 4. SIMD Hot Paths

`strings.IndexByte` is used everywhere a single-character delimiter must be found:

- Scanning text content to the next `<`
- Finding the closing `]]>` of CDATA sections
- Locating the closing quote of attribute values

On modern CPUs this processes **16–32 bytes per cycle**, which is why large-document
throughput exceeds 7 GB/s.

### 5. TypeArray Promotion (O(1))

Repeated sibling elements are merged into a synthetic `TypeArray` node **during
parsing**, with a single O(1) name comparison after each child append. No second
pass is needed.

```
<root>          →  root.children = [TypeArray{name:"item", children:[a,b,c]}]
  <item>a</item>
  <item>b</item>
  <item>c</item>
</root>
```

`MarshalTo` renders `TypeArray` as a JSON array directly.

### 6. MarshalTo Append Pattern

```go
dst = v.MarshalTo(dst[:0])   // caller owns the buffer, reuses it every call
```

No intermediate `strings.Builder`, no `fmt.Sprintf`, no per-call allocation.
Raw attribute values and text sub-slices are appended verbatim when they contain
no characters that need JSON escaping (fast-path `appendJSONString`).

### 7. ParserPool

```go
var pool fastxml.ParserPool

p := pool.Get()
v, _ := p.Parse(xml)
// use v ...
pool.Put(p)   // slab buffers are retained inside p, reused next Get()
```

Each goroutine gets its own `Parser` with its own slab, so there is no lock
contention on the hot path. `sync.Pool` handles the recycling.

## API Reference

```go
// Parser — not safe for concurrent use; use ParserPool instead
func (p *Parser) Parse(s string) (*Value, error)
func (p *Parser) ParseBytes(b []byte) (*Value, error)

// Value navigation
func (v *Value) Type() Type
func (v *Value) Name() string
func (v *Value) Text() string
func (v *Value) GetAttr(name string) string
func (v *Value) Get(path ...string) *Value
func (v *Value) Children(tag string) []*Value

// JSON serialization
func (v *Value) MarshalTo(dst []byte) []byte

// Concurrency
type ParserPool struct{ ... }
func (pp *ParserPool) Get() *Parser
func (pp *ParserPool) Put(p *Parser)

// Programmatic construction
type Arena struct{ ... }
func (a *Arena) Reset()
func (a *Arena) NewElement(name string) *Value
func (a *Arena) NewText(text string) *Value
func (a *Arena) AddChild(parent, child *Value)
func (a *Arena) SetAttr(v *Value, name, value string)
```

## Installation

```bash
go get github.com/donge/fastxml
```

## Running Tests and Benchmarks

```bash
go test ./...
go test -bench=. -benchmem -benchtime=5s ./...
```

## Comparison with Other Libraries

Benchmark source: [donge/xmlbench](https://github.com/donge/xmlbench)  
Hardware: Apple M4 · Go 1.22 · input: real-world business XML (~900 B, medium complexity, with attributes + nested elements + repeated children)

### XML → JSON only

| Library | ns/op | B/op | allocs/op | vs fastxml |
|---|---|---|---|---|
| [basgys/goxml2json](https://github.com/basgys/goxml2json) (baseline) | ~9,800 | ~8,400 | 174 | 18× slower |
| goxml2json (optimized, pooled) | ~1,100 | ~2,100 | 18 | 2× slower |
| [orisano/gosax](https://github.com/orisano/gosax) (stream) | ~780 | ~66,000 | 5 | 1.4× slower, 70× more B/op |
| [clbanning/mxj/v2](https://github.com/clbanning/mxj) | ~12,000 | ~12,800 | 210 | 22× slower |
| **fastxml** | **~545** | **~896** | **1** | — |

### End-to-end: gzip decompress + XML → JSON

| Library | ns/op | B/op | allocs/op |
|---|---|---|---|
| goxml2json (baseline) | ~17,200 | ~9,700 | 185 |
| goxml2json (optimized) | ~7,400 | ~3,400 | 29 |
| gosax | ~7,100 | ~67,300 | 16 |
| mxj/v2 | ~19,500 | ~14,100 | 221 |
| **fastxml** | **~6,800** | **~2,200** | **12** |

> **Note:** In the end-to-end case, gzip decompression (~6 µs) dominates total time. fastxml's XML→JSON step takes only ~300 ns — so switching the XML library alone trims ~4% of total latency; the bigger wins come from pooling the gzip reader (~15% CPU saved) and other pipeline optimizations.

### Why fastxml wins

| Factor | Other libraries | fastxml |
|---|---|---|
| Intermediate representation | `map[string]interface{}` (mxj), DOM tree with allocs | Slab-allocated Value tree, all strings are sub-slices |
| String copies | Multiple per node | 1 (input copy into `p.b`), zero thereafter |
| Entity unescaping | Eager, always allocates | Lazy; skipped entirely if no `&` present |
| Repeated elements | Collected into slices post-parse | Promoted to TypeArray in O(1) during parse |
| JSON serialization | `encoding/json` on intermediate map | Direct `MarshalTo` append, no reflection |
| Allocation pattern | Hundreds of small heap objects | Two growing slabs, steady-state 0–1 allocs/op |

Reproduce:

```bash
git clone https://github.com/donge/xmlbench
cd xmlbench
go test -bench='Xml2Json|DecompressThen' -benchmem ./...
```

## License

MIT
