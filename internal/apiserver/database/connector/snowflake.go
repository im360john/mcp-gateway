package connector

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	sf "github.com/snowflakedb/gosnowflake"
)

// SnowflakeConnector implements the DatabaseConnector interface for Snowflake
type SnowflakeConnector struct {
	db     *sqlx.DB
	config *SnowflakeConfig
}

// NewSnowflakeConnector creates a new Snowflake connector
func NewSnowflakeConnector(config *SnowflakeConfig) (DatabaseConnector, error) {
	if config == nil {
		return nil, fmt.Errorf("snowflake configuration is required")
	}
	
	return &SnowflakeConnector{
		config: config,
	}, nil
}

// Connect establishes a connection to the Snowflake database
func (c *SnowflakeConnector) Connect(ctx context.Context) error {
	// Create DSN based on authentication type
	dsn, err := createSnowflakeDSN(c.config)
	if err != nil {
		return fmt.Errorf("failed to create Snowflake DSN: %w", err)
	}

	// Connect to Snowflake
	db, err := sqlx.ConnectContext(ctx, "snowflake", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to Snowflake: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	c.db = db
	return nil
}

// Disconnect closes the database connection
func (c *SnowflakeConnector) Disconnect(ctx context.Context) error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// ListTables returns a list of available tables
func (c *SnowflakeConnector) ListTables(ctx context.Context) ([]Table, error) {
	if c.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
		SELECT 
			table_name,
			table_type
		FROM 
			information_schema.tables
		WHERE 
			table_schema = ?
		ORDER BY 
			table_name
	`

	rows, err := c.db.QueryxContext(ctx, query, c.config.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var tableName, tableType string
		if err := rows.Scan(&tableName, &tableType); err != nil {
			return nil, fmt.Errorf("failed to scan table row: %w", err)
		}

		// Skip views and other non-table objects
		if strings.ToUpper(tableType) != "BASE TABLE" {
			continue
		}

		// Get row count
		rowCount, err := c.getTableRowCount(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get row count for table %s: %w", tableName, err)
		}

		table := Table{
			Name:     tableName,
			RowCount: rowCount,
		}

		tables = append(tables, table)
	}

	return tables, nil
}

// GetTableMetadata retrieves detailed information about a table
func (c *SnowflakeConnector) GetTableMetadata(ctx context.Context, tableName string) (*TableMetadata, error) {
	if c.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	// Get table columns
	columns, err := c.getTableColumns(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Get row count
	rowCount, err := c.getTableRowCount(ctx, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get row count: %w", err)
	}

	// Get sample data
	sampleData, err := c.getTableSampleData(ctx, tableName, columns)
	if err != nil {
		return nil, fmt.Errorf("failed to get sample data: %w", err)
	}

	// Get table description
	var description string
	query := `
		SELECT comment 
		FROM information_schema.tables 
		WHERE table_name = ? 
		AND table_schema = ? 
		AND table_catalog = ?
	`
	err = c.db.GetContext(ctx, &description, query, tableName, c.config.Schema, c.config.Database)
	if err != nil {
		// Not critical, just log and continue
		description = ""
	}

	// Create metadata
	metadata := &TableMetadata{
		Name:        tableName,
		Description: description,
		Columns:     columns,
		SampleData:  sampleData,
		RowCount:    rowCount,
	}

	return metadata, nil
}

// ExecuteQuery runs a SQL query against the database
func (c *SnowflakeConnector) ExecuteQuery(ctx context.Context, query string, params map[string]interface{}) ([]map[string]interface{}, error) {
	if c.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	// Prepare the query with named parameters
	namedQuery, args, err := sqlx.Named(query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare named query: %w", err)
	}
	
	// Convert to ? placeholders for Snowflake
	query, args, err = sqlx.In(namedQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert named parameters: %w", err)
	}
	
	// Execute the query
	rows, err := c.db.QueryxContext(ctx, c.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()
	
	// Process results
	var result []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		result = append(result, row)
	}
	
	return result, nil
}

// GenerateAPIEndpoints creates API endpoints based on database tables
func (c *SnowflakeConnector) GenerateAPIEndpoints(ctx context.Context, tables []string) ([]APIEndpoint, error) {
	if c.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	var endpoints []APIEndpoint

	for _, tableName := range tables {
		// Get table metadata
		metadata, err := c.GetTableMetadata(ctx, tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata for table %s: %w", tableName, err)
		}

		// Find primary key column
		var primaryKeyColumn string
		for _, col := range metadata.Columns {
			if col.PrimaryKey {
				primaryKeyColumn = col.Name
				break
			}
		}

		// Generate endpoints for this table
		tableEndpoints := []APIEndpoint{
			// List all records
			{
				Method:      "GET",
				Path:        fmt.Sprintf("/%s", tableName),
				Description: fmt.Sprintf("List all records from %s table", tableName),
				Query:       fmt.Sprintf("SELECT * FROM \"%s\".\"%s\".\"%s\" LIMIT :limit OFFSET :offset", c.config.Database, c.config.Schema, tableName),
				Parameters: map[string]interface{}{
					"limit":  "Number of records to return",
					"offset": "Number of records to skip",
				},
			},
		}

		// Add get by ID endpoint if primary key exists
		if primaryKeyColumn != "" {
			tableEndpoints = append(tableEndpoints, APIEndpoint{
				Method:      "GET",
				Path:        fmt.Sprintf("/%s/{%s}", tableName, primaryKeyColumn),
				Description: fmt.Sprintf("Get a single record from %s by ID", tableName),
				Query:       fmt.Sprintf("SELECT * FROM \"%s\".\"%s\".\"%s\" WHERE \"%s\" = :%s", c.config.Database, c.config.Schema, tableName, primaryKeyColumn, primaryKeyColumn),
				Parameters: map[string]interface{}{
					primaryKeyColumn: fmt.Sprintf("ID of the %s record", tableName),
				},
			})
		}

		endpoints = append(endpoints, tableEndpoints...)
	}

	return endpoints, nil
}

// EnhanceMetadataWithLLM uses LLM to generate verbose descriptions
func (c *SnowflakeConnector) EnhanceMetadataWithLLM(ctx context.Context, metadata *TableMetadata) error {
	// This is a placeholder for LLM integration
	// In a real implementation, this would call an LLM service
	
	// For now, generate a basic description
	description := fmt.Sprintf("Table %s contains %d columns and %d rows. ", 
		metadata.Name, len(metadata.Columns), metadata.RowCount)
	
	// Add column information
	description += "Columns include: "
	for i, col := range metadata.Columns {
		if i > 0 {
			description += ", "
		}
		description += col.Name + " (" + col.Type + ")"
		if col.PrimaryKey {
			description += " [Primary Key]"
		}
	}
	
	metadata.VerboseDescription = description
	return nil
}

// Helper functions

// getTableColumns retrieves column information for a table
func (c *SnowflakeConnector) getTableColumns(ctx context.Context, tableName string) ([]Column, error) {
	query := `
		SELECT 
			c.COLUMN_NAME,
			c.DATA_TYPE,
			c.COMMENT,
			CASE WHEN k.COLUMN_NAME IS NOT NULL THEN true ELSE false END as is_primary_key
		FROM 
			information_schema.columns c
		LEFT JOIN 
			information_schema.key_column_usage k 
		ON 
			c.table_catalog = k.table_catalog 
			AND c.table_schema = k.table_schema
			AND c.table_name = k.table_name 
			AND c.column_name = k.column_name 
			AND k.constraint_name LIKE 'SYS_CONSTRAINT_%'
		WHERE 
			c.table_name = ?
			AND c.table_schema = ?
			AND c.table_catalog = ?
		ORDER BY 
			c.ordinal_position
	`

	rows, err := c.db.QueryxContext(ctx, query, tableName, c.config.Schema, c.config.Database)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}
	defer rows.Close()

	var columns []Column
	for rows.Next() {
		var name, dataType, comment string
		var isPrimaryKey bool
		if err := rows.Scan(&name, &dataType, &comment, &isPrimaryKey); err != nil {
			return nil, fmt.Errorf("failed to scan column row: %w", err)
		}

		column := Column{
			Name:        name,
			Type:        dataType,
			Description: comment,
			PrimaryKey:  isPrimaryKey,
		}

		columns = append(columns, column)
	}

	return columns, nil
}

// getTableRowCount gets the row count for a table
func (c *SnowflakeConnector) getTableRowCount(ctx context.Context, tableName string) (int, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM "%s"."%s"."%s"`, 
		c.config.Database, c.config.Schema, tableName)
	
	var count int
	err := c.db.GetContext(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("failed to get row count: %w", err)
	}

	return count, nil
}

// getTableSampleData retrieves sample data from a table
func (c *SnowflakeConnector) getTableSampleData(ctx context.Context, tableName string, columns []Column) ([]map[string]interface{}, error) {
	// Build column list for query
	var columnNames []string
	for _, col := range columns {
		columnNames = append(columnNames, fmt.Sprintf(`"%s"`, col.Name))
	}

	// Build query to get sample data (limit to 5 rows)
	query := fmt.Sprintf(`
		SELECT %s 
		FROM "%s"."%s"."%s" 
		LIMIT 5
	`, strings.Join(columnNames, ", "), c.config.Database, c.config.Schema, tableName)

	// Execute query
	rows, err := c.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get sample data: %w", err)
	}
	defer rows.Close()

	// Process results
	var result []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("failed to scan sample data row: %w", err)
		}
		result = append(result, row)
	}

	return result, nil
}

// createSnowflakeDSN creates a DSN string for Snowflake connection
func createSnowflakeDSN(cfg *SnowflakeConfig) (string, error) {
	config := sf.Config{
		Account:   cfg.Account,
		User:      cfg.Username,
		Database:  cfg.Database,
		Schema:    cfg.Schema,
		Warehouse: cfg.Warehouse,
	}

	if cfg.Role != "" {
		config.Role = cfg.Role
	}

	// Set authentication based on type
	switch strings.ToLower(cfg.AuthType) {
	case "password":
		config.Password = cfg.Password
	case "key_pair":
		// Handle private key authentication
		var privateKey *rsa.PrivateKey
		var err error

		if cfg.PrivateKey != "" {
			// Parse private key from string
			privateKey, err = parsePrivateKey([]byte(cfg.PrivateKey))
			if err != nil {
				return "", fmt.Errorf("failed to parse private key: %w", err)
			}
		} else if cfg.PrivateKeyPath != "" {
			// Read private key from file
			keyBytes, err := ioutil.ReadFile(cfg.PrivateKeyPath)
			if err != nil {
				return "", fmt.Errorf("failed to read private key file: %w", err)
			}
			privateKey, err = parsePrivateKey(keyBytes)
			if err != nil {
				return "", fmt.Errorf("failed to parse private key from file: %w", err)
			}
		} else {
			return "", fmt.Errorf("either private_key or private_key_path must be provided for key_pair authentication")
		}

		// Set private key in config
		config.PrivateKey = privateKey
	default:
		return "", fmt.Errorf("unsupported authentication type: %s", cfg.AuthType)
	}

	// Create DSN
	dsn, err := sf.DSN(&config)
	if err != nil {
		return "", fmt.Errorf("failed to create Snowflake DSN: %w", err)
	}

	return dsn, nil
}

// parsePrivateKey parses a PEM encoded private key
func parsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}
