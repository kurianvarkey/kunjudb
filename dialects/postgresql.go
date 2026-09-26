package dialects

import "fmt"

type PostgreSql struct{}

func (p PostgreSql) Placeholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

func (p PostgreSql) Quote(s string) string {
	return fmt.Sprintf("\"%s\"", s)
}
