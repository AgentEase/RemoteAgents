package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/remote-agent-terminal/backend/api/handlers"
	"github.com/remote-agent-terminal/backend/internal/db"
	"github.com/remote-agent-terminal/backend/internal/pty"
	"github.com/remote-agent-terminal/backend/internal/repository"
	"github.com/remote-agent-terminal/backend/internal/session"
	"github.com/remote-agent-terminal/backend/internal/ws"
	"github.com/remote-agent-terminal/backend/pkg/driver"
)

func main() {
	// Get configuration from environment
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "data/sessions.db")
	logDir := getEnv("LOG_DIR", "data/logs")
	maxSessions := 10

	// Ensure data directories exist
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create log directory: %v", err)
	}

	// Initialize database
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.CloseDB()

	// Initialize repository
	sessionRepo := repository.NewSessionRepository(database)

	// Initialize PTY manager
	ptyManager := pty.NewManager(logDir)
	defer ptyManager.Close()

	// Initialize session manager
	sessionManager := session.NewManager(ptyManager, sessionRepo, session.Config{
		LogDir:             logDir,
		MaxSessionsPerUser: maxSessions,
	})
	defer sessionManager.Close()


	// Initialize WebSocket service
	agentDriver := driver.NewGenericDriver()
	wsService := ws.NewService(ptyManager, agentDriver)
	defer wsService.Close()

	// Initialize handlers
	sessionHandler := handlers.NewSessionHandler(sessionManager)
	wsHandler := handlers.NewWebSocketHandler(sessionManager, wsService.Handler())

	// Initialize Gin router
	r := gin.Default()

	// Enable CORS for development
	r.Use(corsMiddleware())

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	// API routes
	api := r.Group("/api")
	{
		// Session management routes
		sessionHandler.RegisterRoutes(api)
		sessionHandler.RegisterLogsRoute(api)

		// WebSocket routes
		wsHandler.RegisterRoutes(api)
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down server...")
		sessionManager.Close()
		ptyManager.Close()
		wsService.Close()
		db.CloseDB()
		os.Exit(0)
	}()

	// Start server
	log.Printf("Starting server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// corsMiddleware returns a CORS middleware for development.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
