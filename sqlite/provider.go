package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite" // SQLite driver
)

// DatabaseService holds the database connection.
type DatabaseService struct {
	db *sql.DB
}

// NewDatabaseService creates a new DatabaseService and connects to the SQLite DB.
func NewDatabaseService(dbFile string) (*DatabaseService, error) {
	if dbFile == "" {
		return nil, fmt.Errorf("DB_FILE environment variable not set")
	}

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", dbFile, err)
	}

	// Check the connection
	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database %s: %w", dbFile, err)
	}

	log.Printf("Successfully connected to database: %s", dbFile)
	return &DatabaseService{db: db}, nil
}

// Close closes the database connection.
func (ds *DatabaseService) Close() error {
	if ds.db != nil {
		log.Println("Closing database connection...")
		return ds.db.Close()
	}
	return nil
}

// readQueryHandler is the handler function for the 'read_query' tool.
func (ds *DatabaseService) readQueryHandler(ctx context.Context, query string) (*mcp.CallToolResult, error) {
	// --- Read-Only Validation ---
	trimmedQuery := strings.TrimSpace(strings.ToUpper(query))
	if !strings.HasPrefix(trimmedQuery, "SELECT") {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Only SELECT queries are allowed for read-only access."},
			},
			IsError: true,
		}, nil
	}
	// More robust validation could be added here if needed (e.g., disallowing PRAGMA, ATTACH etc.)

	// --- Execute Query ---
	rows, err := ds.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error executing query: %v, Query: %s", err, query)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error executing query: %v", err)},
			},
			IsError: true,
		}, nil
	}
	defer rows.Close()

	// --- Process Results ---
	return processRows(rows) // Use helper function
}

// listTablesHandler lists all user tables in the database.
func (ds *DatabaseService) listTablesHandler(ctx context.Context) (*mcp.CallToolResult, error) {
	query := "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;"
	rows, err := ds.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error listing tables: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error listing tables: %v", err)},
			},
			IsError: true,
		}, nil
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("Error scanning table name: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error reading table name: %v", err)},
				},
				IsError: true,
			}, nil
		}
		tables = append(tables, name)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating table list: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error iterating through table list: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// Format result as JSON array string
	resultJSON, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		log.Printf("Error marshalling table list to JSON: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error formatting table list: %v", err)},
			},
			IsError: true,
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(resultJSON)},
		},
	}, nil
}

// describeTableHandler provides schema information for a specific table.
func (ds *DatabaseService) describeTableHandler(ctx context.Context, tableName string) (*mcp.CallToolResult, error) {
	// Basic validation to prevent SQL injection in PRAGMA
	// A stricter validation (e.g., checking against list_tables result) is recommended for production
	if strings.ContainsAny(tableName, "';--") {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Invalid characters in table name."},
			},
			IsError: true,
		}, nil
	}

	// Use PRAGMA table_info with properly quoted table name to handle spaces and special characters
	// Quote the table name with double quotes to handle spaces and other special characters
	query := fmt.Sprintf("PRAGMA table_info(\"%s\");", strings.ReplaceAll(tableName, "\"", "\"\""))

	rows, err := ds.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error describing table %s: %v", tableName, err)
		// Check if the error is because the table doesn't exist
		// Note: The specific error message might vary depending on the driver/SQLite version
		if strings.Contains(err.Error(), "no such table") || strings.Contains(err.Error(), "unable to use function") {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Table '%s' not found or PRAGMA query failed.", tableName)},
				},
				IsError: true,
			}, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error describing table '%s': %v", tableName, err)},
			},
			IsError: true,
		}, nil
	}
	defer rows.Close()
	return processRows(rows) // Use helper function to format PRAGMA results
}

// updateHandler is a fake update handler that does nothing but accepts parameters.
func (ds *DatabaseService) updateHandler(_, _, _ string) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Update command received but not executed (read-only mode)"},
		},
	}, nil
}

// processRows is a helper function to process sql.Rows into a CallToolResult.
func processRows(rows *sql.Rows) (*mcp.CallToolResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Error getting columns: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error getting result columns: %v", err)},
			},
			IsError: true,
		}, nil
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		log.Printf("Error getting column types: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error getting result column types: %v", err)},
			},
			IsError: true,
		}, nil
	}

	results := []map[string]interface{}{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			log.Printf("Error scanning row: %v", err)
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Error reading result row: %v", err)},
				},
				IsError: true,
			}, nil
		}

		rowMap := make(map[string]interface{})
		for i, colName := range columns {
			// Handle potential NULL values and different data types gracefully
			val := values[i]
			if val == nil {
				rowMap[colName] = nil
				continue
			}

			// Try to retain original type if possible, fallback to string representation
			switch v := val.(type) {
			case []byte:
				colType := columnTypes[i].DatabaseTypeName()
				if strings.Contains(strings.ToUpper(colType), "BLOB") {
					rowMap[colName] = fmt.Sprintf("BLOB data (length %d)", len(v)) // Avoid sending large blobs directly
				} else {
					rowMap[colName] = string(v) // Assume text if not explicitly BLOB
				}
			case int64, float64, bool, string:
				rowMap[colName] = v
			// Handle specific types returned by PRAGMA table_info if needed
			// (e.g., 'pk' which might be int64 0 or 1)
			default:
				// Convert integer types specifically if needed by the client
				if iType, ok := val.(int); ok {
					rowMap[colName] = int64(iType)
				} else if iType32, ok := val.(int32); ok {
					rowMap[colName] = int64(iType32)
				} else {
					rowMap[colName] = fmt.Sprintf("%v", v) // Fallback representation
				}
			}
		}
		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error iterating through results: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// --- Format Output ---
	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("Error marshalling results to JSON: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Error formatting results: %v", err)},
			},
			IsError: true,
		}, nil
	}

	// Limit the size of the output to avoid overly large responses
	const maxResultSize = 10000 // Limit to ~10KB, adjust as needed
	resultStr := string(resultJSON)
	if len(resultStr) > maxResultSize {
		resultStr = resultStr[:maxResultSize] + "\n... (results truncated)"
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: resultStr},
		},
	}, nil
}

func NewServer(ctx context.Context, env map[string]string) (*mcp.Server, error) {
	dbFile, ok := env["DB_FILE"]
	if !ok || dbFile == "" {
		return nil, fmt.Errorf("DB_FILE environment variable not set or empty")
	}

	// Initialize Database Service
	dbService, err := NewDatabaseService(dbFile)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	go func() {
		<-ctx.Done()
		dbService.Close()
	}()

	// Create MCP Server
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "sqlite-readonly",
			Version: "1.0.0",
		},
		nil,
	)

	// Define tool argument types
	type readQueryArgs struct {
		Query string `json:"query" jsonschema:"The SELECT SQL query to execute"`
	}
	type describeTableArgs struct {
		TableName string `json:"table_name" jsonschema:"Name of the table to describe"`
	}
	type updateArgs struct {
		TableName   string `json:"table_name" jsonschema:"Name of the table to update"`
		SetClause   string `json:"set_clause" jsonschema:"SET clause for the update (e.g. 'name=John, age=30')"`
		WhereClause string `json:"where_clause" jsonschema:"WHERE clause to filter which records to update (e.g. 'id=1')"`
	}

	// Add read_query tool
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "read_query",
		Description: "Execute a read-only SELECT query on the SQLite database",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args readQueryArgs) (*mcp.CallToolResult, any, error) {
		result, err := dbService.readQueryHandler(ctx, args.Query)
		return result, nil, err
	})

	// Add list_tables tool
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_tables",
		Description: "List all user tables in the SQLite database",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
		result, err := dbService.listTablesHandler(ctx)
		return result, nil, err
	})

	// Add describe_table tool
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "describe_table",
		Description: "Get the schema information (columns, types) for a specific table",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args describeTableArgs) (*mcp.CallToolResult, any, error) {
		result, err := dbService.describeTableHandler(ctx, args.TableName)
		return result, nil, err
	})

	// Add update tool (fake, does nothing - only to demonstrate tool blocking by PPL)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "update",
		Description: "Update records in a table",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args updateArgs) (*mcp.CallToolResult, any, error) {
		result, err := dbService.updateHandler(args.TableName, args.SetClause, args.WhereClause)
		return result, nil, err
	})

	return mcpServer, nil
}
