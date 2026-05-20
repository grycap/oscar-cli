package tui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func setServiceTableHeader(table *tview.Table) {
	setTableHeader(table, serviceHeaders)
}

func setLogTableHeader(table *tview.Table) {
	setTableHeader(table, logHeaders)
}

func setBucketTableHeader(table *tview.Table) {
	setTableHeader(table, bucketHeaders)
}

func setBucketObjectTableHeader(table *tview.Table) {
	setTableHeader(table, bucketObjectHeaders)
}

func setTableHeader(table *tview.Table, headers []string) {
	table.Clear()
	for col, header := range headers {
		table.SetCell(0, col, tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold))
	}
}

func fillMessageRow(table *tview.Table, columns int, message string) {
	table.SetCell(1, 0, tview.NewTableCell(message).
		SetAlign(tview.AlignCenter).
		SetSelectable(false).
		SetExpansion(columns))
	for col := 1; col < columns; col++ {
		table.SetCell(1, col, tview.NewTableCell("").SetSelectable(false))
	}
}

func bucketVisibilityColor(vis string) tcell.Color {
	switch strings.ToLower(strings.TrimSpace(vis)) {
	case "restricted":
		return tcell.ColorYellow
	case "private":
		return tcell.ColorRed
	case "public":
		return tcell.ColorGreen
	default:
		return tcell.ColorWhite
	}
}
