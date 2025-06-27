package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
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
func (ds *DatabaseService) readQueryHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("Missing or invalid 'query' argument."), nil
	}

	// --- Read-Only Validation ---
	trimmedQuery := strings.TrimSpace(strings.ToUpper(query))
	if !strings.HasPrefix(trimmedQuery, "SELECT") {
		return mcp.NewToolResultError("Only SELECT queries are allowed for read-only access."), nil
	}
	// More robust validation could be added here if needed (e.g., disallowing PRAGMA, ATTACH etc.)

	// --- Execute Query ---
	rows, err := ds.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error executing query: %v, Query: %s", err, query)
		return mcp.NewToolResultErrorFromErr("Error executing query", err), nil
	}
	defer rows.Close()

	// --- Process Results ---
	return processRows(rows) // Use helper function
}

// listTablesHandler lists all user tables in the database.
func (ds *DatabaseService) listTablesHandler(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := "SELECT name FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;"
	rows, err := ds.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("Error listing tables: %v", err)
		return mcp.NewToolResultErrorFromErr("Error listing tables", err), nil
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("Error scanning table name: %v", err)
			return mcp.NewToolResultErrorFromErr("Error reading table name", err), nil
		}
		tables = append(tables, name)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating table list: %v", err)
		return mcp.NewToolResultErrorFromErr("Error iterating through table list", err), nil
	}

	// Format result as JSON array string
	resultJSON, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		log.Printf("Error marshalling table list to JSON: %v", err)
		return mcp.NewToolResultErrorFromErr("Error formatting table list", err), nil
	}

	return mcp.NewToolResultText(string(resultJSON)), nil
}

// describeTableHandler provides schema information for a specific table.
func (ds *DatabaseService) describeTableHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	tableName, ok := args["table_name"].(string)
	if !ok || tableName == "" {
		return mcp.NewToolResultError("Missing or invalid 'table_name' argument."), nil
	}

	// Basic validation to prevent SQL injection in PRAGMA
	// A stricter validation (e.g., checking against list_tables result) is recommended for production
	if strings.ContainsAny(tableName, "';--") {
		return mcp.NewToolResultError("Invalid characters in table name."), nil
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
			return mcp.NewToolResultError(fmt.Sprintf("Table '%s' not found or PRAGMA query failed.", tableName)), nil
		}
		return mcp.NewToolResultErrorFromErr(fmt.Sprintf("Error describing table '%s'", tableName), err), nil
	}
	defer rows.Close()

	return processRows(rows) // Use helper function to format PRAGMA results
}

// processRows is a helper function to process sql.Rows into a CallToolResult.
func processRows(rows *sql.Rows) (*mcp.CallToolResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		log.Printf("Error getting columns: %v", err)
		return mcp.NewToolResultErrorFromErr("Error getting result columns", err), nil
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		log.Printf("Error getting column types: %v", err)
		return mcp.NewToolResultErrorFromErr("Error getting result column types", err), nil
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
			return mcp.NewToolResultErrorFromErr("Error reading result row", err), nil
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
		return mcp.NewToolResultErrorFromErr("Error iterating through results", err), nil
	}

	// --- Format Output ---
	resultJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("Error marshalling results to JSON: %v", err)
		return mcp.NewToolResultErrorFromErr("Error formatting results", err), nil
	}

	// Limit the size of the output to avoid overly large responses
	const maxResultSize = 10000 // Limit to ~10KB, adjust as needed
	resultStr := string(resultJSON)
	if len(resultStr) > maxResultSize {
		resultStr = resultStr[:maxResultSize] + "\n... (results truncated)"
	}

	return mcp.NewToolResultText(resultStr), nil
}

func NewServer(ctx context.Context, env map[string]string) (*server.MCPServer, error) {
	dbFile, ok := env["DB_FILE"]
	if !ok || dbFile == "" {
		return nil, fmt.Errorf("DB_FILE environment variable not set or empty")
	}

	// Initialize Database Service
	dbService, err := NewDatabaseService(dbFile)
	if err != nil {
		log.Fatalf("Failed to initialize database service: %v", err)
	}
	go func() {
		<-ctx.Done()
		dbService.Close()
	}()

	// Create MCP Server
	mcpServer := server.NewMCPServer(
		"sqlite-readonly",
		"1.0.0",
		server.WithToolCapabilities(true), // Enable tools
		server.WithLogging(),              // Enable basic logging via MCP
		server.WithRecovery(),             // Add panic recovery middleware
	)

	// --- Define Tools ---

	// 1. read_query tool
	readQueryTool := mcp.NewTool(
		"read_query",
		mcp.WithDescription("Execute a read-only SELECT query on the SQLite database"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("The SELECT SQL query to execute"),
		),
	)
	mcpServer.AddTool(readQueryTool, dbService.readQueryHandler)

	// 2. list_tables tool
	listTablesTool := mcp.NewTool(
		"list_tables",
		mcp.WithDescription("List all user tables in the SQLite database"),
	)
	mcpServer.AddTool(listTablesTool, dbService.listTablesHandler)

	// 3. describe_table tool
	describeTableTool := mcp.NewTool(
		"describe_table",
		mcp.WithDescription("Get the schema information (columns, types) for a specific table"),
		mcp.WithString("table_name",
			mcp.Required(),
			mcp.Description("Name of the table to describe"),
		),
	)
	mcpServer.AddTool(describeTableTool, dbService.describeTableHandler)

	return mcpServer, nil
}
