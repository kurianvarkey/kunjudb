package dialects

type Dialect interface {
	Placeholder(n int) string // Returns "?" or "$n"
	Quote(s string) string
}

type placeholderTracker struct {
	count   int
	dialect Dialect
}

// Next returns the next placeholder
func (p *placeholderTracker) Next() string {
	p.count++
	return p.dialect.Placeholder(p.count)
}
