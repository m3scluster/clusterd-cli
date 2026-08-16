package table

import (
	"fmt"
	"strings"
	"unicode"
)

type Table struct {
	rows    [][]string
	padding []int
}

func hasTripleWhitespace(value string) bool {
	var previous rune
	count := 0
	for _, current := range value {
		if unicode.IsSpace(current) && current == previous {
			count++
		} else {
			count = 1
		}
		if unicode.IsSpace(current) && count >= 3 {
			return true
		}
		previous = current
	}
	return false
}

func New(columns []string) (*Table, error) {
	if columns == nil {
		return nil, fmt.Errorf("Column headers must be supplied as a list")
	}
	for _, column := range columns {
		if hasTripleWhitespace(column) {
			return nil, fmt.Errorf("Column headers cannot have more than one space between words")
		}
	}
	padding := make([]int, len(columns))
	for i, column := range columns {
		padding[i] = len(column)
	}
	return &Table{rows: [][]string{append([]string(nil), columns...)}, padding: padding}, nil
}

func (t *Table) AddRow(row []string) error {
	if len(row) != len(t.rows[0]) {
		return fmt.Errorf("Number of entries and columns do not match!")
	}
	copyRow := append([]string(nil), row...)
	for i, value := range copyRow {
		if len(value) > t.padding[i] {
			t.padding[i] = len(value)
		}
	}
	t.rows = append(t.rows, copyRow)
	return nil
}

func (t *Table) String() string {
	var b strings.Builder
	for rowIndex, row := range t.rows {
		for columnIndex, value := range row {
			b.WriteString(value)
			b.WriteString(strings.Repeat(" ", t.padding[columnIndex]-len(value)+2))
		}
		if rowIndex != len(t.rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
