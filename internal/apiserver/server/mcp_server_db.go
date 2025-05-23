package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/mcp-ecosystem/mcp-gateway/internal/apiserver/database/connector"
)

// MCPServerConfig extends the existing configuration with database options
type MCPServerConfig struct {
	// Existing fields
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	Preinstalled bool             `json:"preinstalled,omitempty"`
	Policy      string            `json:"policy,omitempty"`
	
	// New database fields
	Database    *connector.DatabaseConfig `json:"database,omitempty"`
	EnableAPI   bool                      `json:"enable_api,omitempty"`
	APIPrefix   string                    `json:"api_prefix,omitempty"`
	EnableLLM   bool                      `json:"enable_llm,omitempty"`
}

// MCPServerWithDB extends the MCP server with database capabilities
type MCPServerWithDB struct {
	Config    *MCPServerConfig
	DBConn    connector.DatabaseConnector
	APIRouter *gin.Engine
	
	// For managing the lifecycle
	ctx        context.Context
	cancelFunc context.CancelFunc
	mutex      sync.Mutex
	isRunning  bool
}

// NewMCPServerWithDB creates a new MCP server with database capabilities
func NewMCPServerWithDB(config *MCPServerConfig) (*MCPServerWithDB, error) {
	if config == nil {
		return nil, fmt.Errorf("server configuration is required")
	}
	
	// Create context with cancel function for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	
	server := &MCPServerWithDB{
		Config:     config,
		ctx:        ctx,
		cancelFunc: cancel,
	}
	
	// Initialize database connector if configured
	if config.Database != nil && config.Database.Type != "" && config.Database.Type != "none" {
		dbConn, err := connector.NewDatabaseConnector(config.Database)
		if err != nil {
			cancel() // Clean up context
			return nil, fmt.Errorf("failed to create database connector: %w", err)
		}
		server.DBConn = dbConn
		
		// Initialize API router if API is enabled
		if config.EnableAPI {
			server.APIRouter = gin.Default()
			
			// Set up API prefix
			apiPrefix := "/api/db"
			if config.APIPrefix != "" {
				apiPrefix = config.APIPrefix
			}
			
			// Initialize API routes
			apiGroup := server.APIRouter.Group(apiPrefix)
			server.setupAPIRoutes(apiGroup)
		}
	}
	
	return server, nil
}

// Start initializes and starts the MCP server
func (s *MCPServerWithDB) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	if s.isRunning {
		return nil // Already running
	}
	
	// Connect to database if configured
	if s.DBConn != nil {
		if err := s.DBConn.Connect(s.ctx); err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		
		// Start API server if enabled
		if s.Config.EnableAPI && s.APIRouter != nil {
			go func() {
				// Use a random port or configured port
				addr := ":8081" // Default port, should be configurable
				log.Printf("Starting API server on %s", addr)
				if err := s.APIRouter.Run(addr); err != nil {
					log.Printf("API server error: %v", err)
				}
			}()
		}
	}
	
	s.isRunning = true
	return nil
}

// Stop gracefully shuts down the MCP server
func (s *MCPServerWithDB) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	if !s.isRunning {
		return nil // Already stopped
	}
	
	// Disconnect from database if connected
	if s.DBConn != nil {
		if err := s.DBConn.Disconnect(s.ctx); err != nil {
			log.Printf("Error disconnecting from database: %v", err)
		}
	}
	
	// Cancel context to signal shutdown
	s.cancelFunc()
	
	s.isRunning = false
	return nil
}

// setupAPIRoutes configures the API routes for database operations
func (s *MCPServerWithDB) setupAPIRoutes(router *gin.RouterGroup) {
	// List tables endpoint
	router.GET("/tables", func(c *gin.Context) {
		tables, err := s.DBConn.ListTables(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list tables: %v", err)})
			return
		}
		c.JSON(http.StatusOK, tables)
	})
	
	// Get table metadata endpoint
	router.GET("/tables/:tableName", func(c *gin.Context) {
		tableName := c.Param("tableName")
		metadata, err := s.DBConn.GetTableMetadata(c.Request.Context(), tableName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get table metadata: %v", err)})
			return
		}
		
		// Enhance metadata with LLM if enabled
		if s.Config.EnableLLM {
			if err := s.DBConn.EnhanceMetadataWithLLM(c.Request.Context(), metadata); err != nil {
				log.Printf("Warning: Failed to enhance metadata with LLM: %v", err)
				// Continue anyway, this is not critical
			}
		}
		
		c.JSON(http.StatusOK, metadata)
	})
	
	// Execute query endpoint
	router.POST("/query", func(c *gin.Context) {
		var request struct {
			Query  string                 `json:"query"`
			Params map[string]interface{} `json:"params"`
		}
		
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request: %v", err)})
			return
		}
		
		results, err := s.DBConn.ExecuteQuery(c.Request.Context(), request.Query, request.Params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to execute query: %v", err)})
			return
		}
		
		c.JSON(http.StatusOK, results)
	})
	
	// Generate API endpoints
	router.POST("/generate-api", func(c *gin.Context) {
		var request struct {
			Tables []string `json:"tables"`
		}
		
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request: %v", err)})
			return
		}
		
		endpoints, err := s.DBConn.GenerateAPIEndpoints(c.Request.Context(), request.Tables)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to generate API endpoints: %v", err)})
			return
		}
		
		// Register the generated endpoints
		s.registerGeneratedEndpoints(router, endpoints)
		
		c.JSON(http.StatusOK, endpoints)
	})
}

// registerGeneratedEndpoints dynamically registers the generated API endpoints
func (s *MCPServerWithDB) registerGeneratedEndpoints(router *gin.RouterGroup, endpoints []connector.APIEndpoint) {
	for _, endpoint := range endpoints {
		// Create a closure to capture the endpoint
		handler := func(endpoint connector.APIEndpoint) gin.HandlerFunc {
			return func(c *gin.Context) {
				// Extract parameters from path and query
				params := make(map[string]interface{})
				
				// Path parameters
				for param := range endpoint.Parameters {
					if value, exists := c.Params.Get(param); exists {
						params[param] = value
					}
				}
				
				// Query parameters
				for key, value := range c.Request.URL.Query() {
					if len(value) > 0 {
						params[key] = value[0]
					}
				}
				
				// Execute the query
				results, err := s.DBConn.ExecuteQuery(c.Request.Context(), endpoint.Query, params)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to execute query: %v", err)})
					return
				}
				
				c.JSON(http.StatusOK, results)
			}
		}(endpoint)
		
		// Register the endpoint with the router
		path := endpoint.Path
		switch endpoint.Method {
		case "GET":
			router.GET(path, handler)
		case "POST":
			router.POST(path, handler)
		case "PUT":
			router.PUT(path, handler)
		case "DELETE":
			router.DELETE(path, handler)
		default:
			log.Printf("Unsupported HTTP method: %s", endpoint.Method)
		}
	}
}

// GetServerInfo returns information about the server
func (s *MCPServerWithDB) GetServerInfo() map[string]interface{} {
	info := map[string]interface{}{
		"name":       s.Config.Name,
		"type":       s.Config.Type,
		"is_running": s.isRunning,
	}
	
	if s.Config.Database != nil {
		info["database"] = map[string]interface{}{
			"type":       s.Config.Database.Type,
			"enable_api": s.Config.EnableAPI,
			"enable_llm": s.Config.EnableLLM,
		}
	}
	
	return info
}

// ToJSON serializes the server configuration to JSON
func (s *MCPServerWithDB) ToJSON() ([]byte, error) {
	return json.Marshal(s.Config)
}

// FromJSON deserializes the server configuration from JSON
func FromJSON(data []byte) (*MCPServerConfig, error) {
	var config MCPServerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal server configuration: %w", err)
	}
	return &config, nil
}
