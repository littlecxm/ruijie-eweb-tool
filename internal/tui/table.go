package tui

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// WriteTable keeps the complete table width; the results viewport can pan it.
func WriteTable(out io.Writer, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	clean := make([][]string, len(rows))
	for i, row := range rows {
		clean[i] = make([]string, len(row))
		for j, cell := range row {
			clean[i][j] = safeText(cell)
		}
	}
	t := table.New().Headers(clean[0]...).Rows(clean[1:]...).Border(lipgloss.NormalBorder()).StyleFunc(func(row, col int) lipgloss.Style {
		return lipgloss.NewStyle().Padding(0, 1)
	})
	_, err := fmt.Fprintln(out, t.Render())
	return err
}
