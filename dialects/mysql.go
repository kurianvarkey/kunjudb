package dialects

import "fmt"

type MySql struct{}

func (m MySql) Placeholder(n int) string {
	return "?"
}

func (m MySql) Quote(s string) string {
	return fmt.Sprintf("`%s`", s)
}
