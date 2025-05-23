package database

import (
	"context"
	"fmt"
)

// DatabaseConnector defines the interface for database operations in MCP servers
type DatabaseConnector interface {
	// Connect establishes a connection to the database
	Connect(ctx context.Context) error
	
	// Disconnect closes the database connection
	Disconnect(ctx context.Context) error
	
	// ListTables returns a list of available tables
	ListTables(ctx context.Context) ([]Table, error)
	
	// GetTableMetadata retrieves detailed information about a table
	GetTableMetadata(ctx context.Context, tableName string) (*TableMetadata, error)
	
	// ExecuteQuery runs a SQL query against the database
	ExecuteQuery(ctx context.Context, query string, params map[string]interface{}) ([]map[string]interface{}, error)
	
	// GenerateAPIEndpoints creates API endpoints based on database tables
	GenerateAPIEndpoints(ctx context.Context, tables []string) ([]APIEndpoint, error)
	
	// EnhanceMetadataWithLLM uses LLM to generate verbose descriptions
	EnhanceMetadataWithLLM(ctx context.Context, metadata *TableMetadata) error
}

// Table represents a database table
type Table struct {
	Name     string `json:"name"`
	RowCount int    `json:"row_count"`
}

// Column represents a database column
type Column struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	PrimaryKey  bool        `json:"primary_key,omitempty"`
	ForeignKey  bool        `json:"foreign_key,omitempty"`
	References  string      `json:"references,omitempty"`
	Sample      interface{} `json:"sample,omitempty"`
}

// TableMetadata contains enhanced metadata for a table
type TableMetadata struct {
	Name              string                   `json:"name"`
	Description       string                   `json:"description,omitempty"`
	Columns           []Column                 `json:"columns"`
	SampleData        []map[string]interface{} `json:"sample_data,omitempty"`
	RowCount          int                      `json:"row_count"`
	VerboseDescription string                  `json:"verbose_description,omitempty"`
}

// APIEndpoint represents a generated API endpoint
type APIEndpoint struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Description string                 `json:"description"`
	Query       string                 `json:"query"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// DatabaseConfig holds the configuration for database connections
type DatabaseConfig struct {
	// Type of database (snowflake, postgres, etc.)
	Type string `json:"type"`
	
	// Specific configuration for each database type
	Snowflake *SnowflakeConfig `json:"snowflake,omitempty"`
	// Other database types can be added here
}

// SnowflakeConfig holds Snowflake-specific configuration
type SnowflakeConfig struct {
	Account        string `json:"account"`
	Username       string `json:"username"`
	Password       string `json:"password,omitempty"`
	Database       string `json:"database"`
	Schema         string `json:"schema"`
	Warehouse      string `json:"warehouse"`
	Role           string `json:"role,omitempty"`
	PrivateKey     string `json:"private_key,omitempty"`
	PrivateKeyPath string `json:"private_key_path,omitempty"`
	AuthType       string `json:"auth_type"` // "password" or "key_pair"
}

// Factory for creating database connectors
func NewDatabaseConnector(config *DatabaseConfig) (DatabaseConnector, error) {
	if config == nil {
		return nil, fmt.Errorf("database configuration is required")
	}

	switch config.Type {
	case "snowflake":
		return NewSnowflakeConnector(config.Snowflake)
	// Other database types can be added here
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.Type)
	}
}
