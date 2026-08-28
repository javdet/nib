package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const CreateTableToolName = "create_table"

var createTableParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "title": {
      "type": "string",
      "description": "Optional heading shown above the table."
    },
    "columns": {
      "type": "array",
      "items": { "type": "string" },
      "minItems": 1,
      "description": "Column headers, left to right."
    },
    "rows": {
      "type": "array",
      "items": {
        "type": "array",
        "items": { "type": "string" }
      },
      "description": "Rows of cells aligned to columns. Use empty string for empty cells. A cell may contain multiple lines."
    }
  },
  "required": ["columns", "rows"]
}`)

// CreateTableToolDef returns the LLM tool definition for rendering a table in chat.
func CreateTableToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: CreateTableToolName,
		Description: "Display a formatted table to the user in the chat. Use for tabular data, comparisons, or board layouts (e.g. Jira columns like To Do, In Progress, Test, Done with task names in the matching column). Do not repeat the table as plain text in your reply.",
		Parameters:  createTableParameters,
	}
}

type tableData struct {
	Title   string
	Columns []string
	Rows    [][]string
}

func parseCreateTableArgs(args map[string]any) (tableData, error) {
	rawCols, ok := args["columns"]
	if !ok {
		return tableData{}, fmt.Errorf("columns is required")
	}
	colsList, ok := rawCols.([]any)
	if !ok {
		return tableData{}, fmt.Errorf("columns must be an array")
	}
	if len(colsList) == 0 {
		return tableData{}, fmt.Errorf("columns must not be empty")
	}

	columns := make([]string, 0, len(colsList))
	for i, col := range colsList {
		s := strings.TrimSpace(argString(col))
		if s == "" {
			return tableData{}, fmt.Errorf("columns[%d] must be a non-empty string", i)
		}
		columns = append(columns, s)
	}

	title := strings.TrimSpace(argString(args["title"]))

	rawRows, ok := args["rows"]
	if !ok {
		return tableData{}, fmt.Errorf("rows is required")
	}
	rowsList, ok := rawRows.([]any)
	if !ok {
		return tableData{}, fmt.Errorf("rows must be an array")
	}

	rows := make([][]string, 0, len(rowsList))
	for i, rowRaw := range rowsList {
		cellsList, ok := rowRaw.([]any)
		if !ok {
			return tableData{}, fmt.Errorf("rows[%d] must be an array", i)
		}
		row := normalizeTableRow(cellsList, len(columns))
		rows = append(rows, row)
	}

	return tableData{
		Title:   title,
		Columns: columns,
		Rows:    rows,
	}, nil
}

func normalizeTableRow(cells []any, colCount int) []string {
	row := make([]string, colCount)
	for j := 0; j < colCount; j++ {
		if j < len(cells) {
			row[j] = argString(cells[j])
		}
	}
	return row
}

// createTableHandler validates table arguments and confirms rendering to the user.
func (s *ChatService) createTableHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if _, err := parseCreateTableArgs(args); err != nil {
			return err.Error(), nil
		}
		return "Table rendered to the user.", nil
	}
}
