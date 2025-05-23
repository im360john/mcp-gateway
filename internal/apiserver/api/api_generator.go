package api

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mcp-ecosystem/mcp-gateway/internal/apiserver/database/connector"
)

// APIGenerator handles the generation of API endpoints from database schemas
type APIGenerator struct {
	connector connector.DatabaseConnector
	config    *APIGeneratorConfig
}

// APIGeneratorConfig holds configuration for API generation
type APIGeneratorConfig struct {
	EnableLLM       bool   `json:"enable_llm"`
	APIPrefix       string `json:"api_prefix"`
	IncludeMetadata bool   `json:"include_metadata"`
}

// NewAPIGenerator creates a new API generator
func NewAPIGenerator(dbConn connector.DatabaseConnector, config *APIGeneratorConfig) *APIGenerator {
	if config == nil {
		config = &APIGeneratorConfig{
			APIPrefix:       "/api/db",
			IncludeMetadata: true,
		}
	}
	
	return &APIGenerator{
		connector: dbConn,
		config:    config,
	}
}

// GenerateAPIFromTables generates API endpoints for the specified tables
func (g *APIGenerator) GenerateAPIFromTables(ctx context.Context, tables []string) ([]connector.APIEndpoint, error) {
	if g.connector == nil {
		return nil, fmt.Errorf("database connector is not initialized")
	}
	
	// If no tables specified, get all tables
	if len(tables) == 0 {
		tableList, err := g.connector.ListTables(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tables: %w", err)
		}
		
		for _, table := range tableList {
			tables = append(tables, table.Name)
		}
	}
	
	// Generate endpoints for each table
	var allEndpoints []connector.APIEndpoint
	
	for _, tableName := range tables {
		endpoints, err := g.generateEndpointsForTable(ctx, tableName)
		if err != nil {
			log.Printf("Warning: Failed to generate endpoints for table %s: %v", tableName, err)
			continue
		}
		
		allEndpoints = append(allEndpoints, endpoints...)
	}
	
	// Add metadata endpoints if enabled
	if g.config.IncludeMetadata {
		metadataEndpoints := g.generateMetadataEndpoints()
		allEndpoints = append(allEndpoints, metadataEndpoints...)
	}
	
	return allEndpoints, nil
}

// generateEndpointsForTable generates API endpoints for a specific table
func (g *APIGenerator) generateEndpointsForTable(ctx context.Context, tableName string) ([]connector.APIEndpoint, error) {
	// Get table metadata
	metadata, err := g.connector.GetTableMetadata(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for table %s: %w", tableName, err)
	}
	
	// Enhance metadata with LLM if enabled
	if g.config.EnableLLM {
		if err := g.connector.EnhanceMetadataWithLLM(ctx, metadata); err != nil {
			log.Printf("Warning: Failed to enhance metadata with LLM: %v", err)
			// Continue anyway, this is not critical
		}
	}
	
	// Find primary key column
	var primaryKeyColumn string
	for _, col := range metadata.Columns {
		if col.PrimaryKey {
			primaryKeyColumn = col.Name
			break
		}
	}
	
	// Generate endpoints
	var endpoints []connector.APIEndpoint
	
	// Base path for this table
	basePath := fmt.Sprintf("/%s", tableName)
	
	// List endpoint (GET /table)
	listEndpoint := connector.APIEndpoint{
		Method:      "GET",
		Path:        basePath,
		Description: fmt.Sprintf("List records from %s table", tableName),
		Query:       fmt.Sprintf("SELECT * FROM %s LIMIT :limit OFFSET :offset", quoteTableName(tableName)),
		Parameters: map[string]interface{}{
			"limit":  "Number of records to return (default: 100)",
			"offset": "Number of records to skip (default: 0)",
		},
	}
	endpoints = append(endpoints, listEndpoint)
	
	// If primary key exists, add get by ID endpoint
	if primaryKeyColumn != "" {
		getByIdEndpoint := connector.APIEndpoint{
			Method:      "GET",
			Path:        fmt.Sprintf("%s/:%s", basePath, primaryKeyColumn),
			Description: fmt.Sprintf("Get a record from %s by ID", tableName),
			Query:       fmt.Sprintf("SELECT * FROM %s WHERE %s = :%s", quoteTableName(tableName), quoteColumnName(primaryKeyColumn), primaryKeyColumn),
			Parameters: map[string]interface{}{
				primaryKeyColumn: fmt.Sprintf("ID of the %s record", tableName),
			},
		}
		endpoints = append(endpoints, getByIdEndpoint)
		
		// Add delete endpoint
		deleteEndpoint := connector.APIEndpoint{
			Method:      "DELETE",
			Path:        fmt.Sprintf("%s/:%s", basePath, primaryKeyColumn),
			Description: fmt.Sprintf("Delete a record from %s by ID", tableName),
			Query:       fmt.Sprintf("DELETE FROM %s WHERE %s = :%s", quoteTableName(tableName), quoteColumnName(primaryKeyColumn), primaryKeyColumn),
			Parameters: map[string]interface{}{
				primaryKeyColumn: fmt.Sprintf("ID of the %s record to delete", tableName),
			},
		}
		endpoints = append(endpoints, deleteEndpoint)
	}
	
	// Add create endpoint (POST /table)
	createEndpoint := connector.APIEndpoint{
		Method:      "POST",
		Path:        basePath,
		Description: fmt.Sprintf("Create a new record in %s table", tableName),
		Query:       g.generateInsertQuery(tableName, metadata.Columns),
		Parameters:  g.generateColumnParameters(metadata.Columns),
	}
	endpoints = append(endpoints, createEndpoint)
	
	// Add update endpoint if primary key exists (PUT /table/:id)
	if primaryKeyColumn != "" {
		updateEndpoint := connector.APIEndpoint{
			Method:      "PUT",
			Path:        fmt.Sprintf("%s/:%s", basePath, primaryKeyColumn),
			Description: fmt.Sprintf("Update a record in %s table", tableName),
			Query:       g.generateUpdateQuery(tableName, metadata.Columns, primaryKeyColumn),
			Parameters:  g.generateColumnParameters(metadata.Columns),
		}
		endpoints = append(endpoints, updateEndpoint)
	}
	
	return endpoints, nil
}

// generateMetadataEndpoints generates API endpoints for metadata operations
func (g *APIGenerator) generateMetadataEndpoints() []connector.APIEndpoint {
	var endpoints []connector.APIEndpoint
	
	// List tables endpoint
	listTablesEndpoint := connector.APIEndpoint{
		Method:      "GET",
		Path:        "/tables",
		Description: "List all available tables",
		Query:       "", // This will be handled specially in the runtime
		Parameters:  map[string]interface{}{},
	}
	endpoints = append(endpoints, listTablesEndpoint)
	
	// Get table metadata endpoint
	tableMetadataEndpoint := connector.APIEndpoint{
		Method:      "GET",
		Path:        "/tables/:tableName",
		Description: "Get metadata for a specific table",
		Query:       "", // This will be handled specially in the runtime
		Parameters: map[string]interface{}{
			"tableName": "Name of the table",
		},
	}
	endpoints = append(endpoints, tableMetadataEndpoint)
	
	// Custom query endpoint
	customQueryEndpoint := connector.APIEndpoint{
		Method:      "POST",
		Path:        "/query",
		Description: "Execute a custom SQL query",
		Query:       "", // This will be handled specially in the runtime
		Parameters: map[string]interface{}{
			"query":  "SQL query to execute",
			"params": "Parameters for the query",
		},
	}
	endpoints = append(endpoints, customQueryEndpoint)
	
	return endpoints
}

// Helper functions

// generateInsertQuery generates an INSERT query for a table
func (g *APIGenerator) generateInsertQuery(tableName string, columns []connector.Column) string {
	var columnNames []string
	var paramNames []string
	
	for _, col := range columns {
		// Skip auto-increment primary keys for insert
		if col.PrimaryKey && isAutoIncrementType(col.Type) {
			continue
		}
		
		columnNames = append(columnNames, quoteColumnName(col.Name))
		paramNames = append(paramNames, fmt.Sprintf(":%s", col.Name))
	}
	
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoteTableName(tableName),
		strings.Join(columnNames, ", "),
		strings.Join(paramNames, ", "))
}

// generateUpdateQuery generates an UPDATE query for a table
func (g *APIGenerator) generateUpdateQuery(tableName string, columns []connector.Column, primaryKeyColumn string) string {
	var setParts []string
	
	for _, col := range columns {
		// Skip primary key for update set clause
		if col.Name == primaryKeyColumn {
			continue
		}
		
		setParts = append(setParts, fmt.Sprintf("%s = :%s", quoteColumnName(col.Name), col.Name))
	}
	
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = :%s",
		quoteTableName(tableName),
		strings.Join(setParts, ", "),
		quoteColumnName(primaryKeyColumn),
		primaryKeyColumn)
}

// generateColumnParameters generates parameter descriptions for columns
func (g *APIGenerator) generateColumnParameters(columns []connector.Column) map[string]interface{} {
	params := make(map[string]interface{})
	
	for _, col := range columns {
		description := col.Name
		if col.Description != "" {
			description = col.Description
		}
		
		params[col.Name] = description
	}
	
	return params
}

// quoteTableName quotes a table name for SQL
func quoteTableName(tableName string) string {
	return fmt.Sprintf("\"%s\"", tableName)
}

// quoteColumnName quotes a column name for SQL
func quoteColumnName(columnName string) string {
	return fmt.Sprintf("\"%s\"", columnName)
}

// isAutoIncrementType checks if a column type is likely auto-increment
func isAutoIncrementType(columnType string) bool {
	// This is a simplistic check and may need to be database-specific
	lowerType := strings.ToLower(columnType)
	return strings.Contains(lowerType, "identity") ||
		strings.Contains(lowerType, "autoincrement") ||
		strings.Contains(lowerType, "serial")
}
