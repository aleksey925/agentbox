package cli

import (
	"fmt"
	"strings"
)

const defaultColumnGap = 2

// Table renders a formatted table with dynamic column widths.
type Table struct {
	headers []string
	rows    [][]string
	gap     int
}

// NewTable creates a new table with the given headers.
func NewTable(headers ...string) *Table {
	return &Table{
		headers: headers,
		gap:     defaultColumnGap,
	}
}

// AddRow adds a row to the table. The number of values should match the number of headers.
func (t *Table) AddRow(values ...string) {
	t.rows = append(t.rows, values)
}

// Render prints the table to stdout with dynamic column widths.
// Headers are automatically converted to UPPERCASE.
func (t *Table) Render() {
	if len(t.headers) == 0 {
		return
	}

	widths := t.calculateWidths()

	upperHeaders := make([]string, len(t.headers))
	for i, h := range t.headers {
		upperHeaders[i] = strings.ToUpper(h)
	}
	t.printRow(upperHeaders, widths)

	for _, row := range t.rows {
		t.printRow(row, widths)
	}
}

func (t *Table) calculateWidths() []int {
	widths := make([]int, len(t.headers))

	for i, h := range t.headers {
		widths[i] = len(h)
	}

	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	for i := range len(widths) - 1 {
		widths[i] += t.gap
	}

	return widths
}

func (t *Table) printRow(cells []string, widths []int) {
	for i, cell := range cells {
		if i < len(widths) {
			if i == len(widths)-1 {
				fmt.Print(cell)
			} else {
				fmt.Printf("%-*s", widths[i], cell)
			}
		}
	}
	fmt.Println()
}
