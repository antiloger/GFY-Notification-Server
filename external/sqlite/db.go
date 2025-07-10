package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/antiloger/Gfy/config"
	"github.com/antiloger/Gfy/core"
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// SQLiteConfig holds SQLite configuration with env tags
type SQLiteConfig struct {
	// Database file path
	DatabasePath string `env:"SQLITE_DATABASE_PATH" default:"./data/app.db"`

	// Connection settings
	MaxOpenConns    int           `env:"SQLITE_MAX_OPEN_CONNS" default:"10"`
	MaxIdleConns    int           `env:"SQLITE_MAX_IDLE_CONNS" default:"5"`
	ConnMaxLifetime time.Duration `env:"SQLITE_CONN_MAX_LIFETIME" default:"1h"`
	ConnMaxIdleTime time.Duration `env:"SQLITE_CONN_MAX_IDLE_TIME" default:"10m"`

	// SQLite specific settings
	BusyTimeout     time.Duration `env:"SQLITE_BUSY_TIMEOUT" default:"5s"`
	CacheSize       int           `env:"SQLITE_CACHE_SIZE" default:"2000"`    // Number of pages
	JournalMode     string        `env:"SQLITE_JOURNAL_MODE" default:"WAL"`   // WAL, DELETE, TRUNCATE, PERSIST, MEMORY, OFF
	SynchronousMode string        `env:"SQLITE_SYNCHRONOUS" default:"NORMAL"` // OFF, NORMAL, FULL, EXTRA

	// Features
	EnableForeignKeys bool `env:"SQLITE_ENABLE_FOREIGN_KEYS" default:"false"`
	EnableAutoVacuum  bool `env:"SQLITE_ENABLE_AUTO_VACUUM" default:"false"`

	// Migration settings
	MigrationsPath string `env:"SQLITE_MIGRATIONS_PATH" default:"./migrations"`
	AutoMigrate    bool   `env:"SQLITE_AUTO_MIGRATE" default:"false"`
}

// SQLite implements the External interface for SQLite database
type SQLite struct {
	config SQLiteConfig
	db     *sql.DB
	logger core.Logger

	// State tracking
	connected bool
}

// New creates a new SQLite instance with config
func New() *SQLite {
	return &SQLite{
		connected: false,
	}
}

// Setup implements External interface - receives framework utilities
func (s *SQLite) Setup(appCtx core.AppContext) error {
	s.config = config.LoadConfig[SQLiteConfig]()
	s.logger = appCtx.Logger().WithComponent("sqlite")

	s.logger.Info("sqlite setup starting",
		core.Field{"database_path", s.config.DatabasePath},
		core.Field{"journal_mode", s.config.JournalMode})

	// Ensure database directory exists
	if err := s.ensureDatabaseDirectory(); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	s.logger.Info("sqlite setup completed")
	return nil
}

// Start implements External interface - establishes database connection
func (s *SQLite) Start(ctx context.Context) error {
	if s.connected {
		return fmt.Errorf("sqlite already connected")
	}

	s.logger.Info("connecting to sqlite database",
		core.Field{"path", s.config.DatabasePath})

	// Build connection string with SQLite pragmas
	dsn := s.buildConnectionString()

	// Open database connection
	var err error
	s.db, err = sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Configure connection pool
	s.db.SetMaxOpenConns(s.config.MaxOpenConns)
	s.db.SetMaxIdleConns(s.config.MaxIdleConns)
	s.db.SetConnMaxLifetime(s.config.ConnMaxLifetime)
	s.db.SetConnMaxIdleTime(s.config.ConnMaxIdleTime)

	// Test connection
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.db.PingContext(pingCtx); err != nil {
		s.db.Close()
		return fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Configure SQLite settings
	if err := s.configureSQLite(ctx); err != nil {
		s.db.Close()
		return fmt.Errorf("failed to configure sqlite: %w", err)
	}

	// Run migrations if enabled
	if s.config.AutoMigrate {
		if err := s.runMigrations(ctx); err != nil {
			s.logger.Warn("migration failed", core.Field{"error", err})
			// Don't fail startup for migration errors in production
		}
	}

	s.connected = true
	s.logger.Info("sqlite database connected successfully",
		core.Field{"max_open_conns", s.config.MaxOpenConns},
		core.Field{"journal_mode", s.config.JournalMode})

	return nil
}

// Stop implements External interface - closes database connection
func (s *SQLite) Stop(ctx context.Context) error {
	if !s.connected {
		s.logger.Info("sqlite already disconnected")
		return nil
	}

	s.logger.Info("disconnecting from sqlite database")

	if s.db != nil {
		if err := s.db.Close(); err != nil {
			s.logger.Error("error closing sqlite database", core.Field{"error", err})
			return fmt.Errorf("failed to close sqlite database: %w", err)
		}
	}

	s.connected = false
	s.logger.Info("sqlite database disconnected successfully")
	return nil
}

// Health implements External interface - health check
func (s *SQLite) Health(ctx context.Context) error {
	if !s.connected {
		return fmt.Errorf("sqlite not connected")
	}

	if s.db == nil {
		return fmt.Errorf("sqlite database is nil")
	}

	// Perform a simple query to check database health
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var result int
	err := s.db.QueryRowContext(healthCtx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("sqlite health check failed: %w", err)
	}

	if result != 1 {
		return fmt.Errorf("sqlite health check returned unexpected result: %d", result)
	}

	return nil
}

// GetDB returns the underlying sql.DB instance for use by other components
func (s *SQLite) GetDB() *sql.DB {
	return s.db
}

// IsConnected returns true if database is connected
func (s *SQLite) IsConnected() bool {
	return s.connected
}

// GetConfig returns the configuration
func (s *SQLite) GetConfig() SQLiteConfig {
	return s.config
}

// ensureDatabaseDirectory creates the database directory if it doesn't exist
func (s *SQLite) ensureDatabaseDirectory() error {
	dir := filepath.Dir(s.config.DatabasePath)
	if dir == "." {
		return nil // Current directory, no need to create
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory %s: %w", dir, err)
	}

	s.logger.Debug("database directory ensured", core.Field{"directory", dir})
	return nil
}

// buildConnectionString creates SQLite connection string with pragmas
func (s *SQLite) buildConnectionString() string {
	dsn := s.config.DatabasePath

	// Add query parameters for SQLite pragmas
	dsn += "?"

	// Busy timeout
	dsn += fmt.Sprintf("_busy_timeout=%d", int(s.config.BusyTimeout.Milliseconds()))

	// Cache size
	dsn += fmt.Sprintf("&cache=shared&_cache_size=%d", s.config.CacheSize)

	// Foreign keys
	if s.config.EnableForeignKeys {
		dsn += "&_foreign_keys=on"
	}

	// Journal mode
	dsn += fmt.Sprintf("&_journal_mode=%s", s.config.JournalMode)

	// Synchronous mode
	dsn += fmt.Sprintf("&_synchronous=%s", s.config.SynchronousMode)

	return dsn
}

// configureSQLite sets up SQLite-specific settings
func (s *SQLite) configureSQLite(ctx context.Context) error {
	pragmas := []string{}

	// Auto vacuum
	if s.config.EnableAutoVacuum {
		pragmas = append(pragmas, "PRAGMA auto_vacuum = FULL")
	}

	// Additional pragmas for performance
	pragmas = append(pragmas,
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 268435456", // 256MB
	)

	// Execute pragmas
	for _, pragma := range pragmas {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			s.logger.Warn("failed to execute pragma",
				core.Field{"pragma", pragma},
				core.Field{"error", err})
		} else {
			s.logger.Debug("pragma executed", core.Field{"pragma", pragma})
		}
	}

	return nil
}

// runMigrations runs database migrations if migration files exist
func (s *SQLite) runMigrations(ctx context.Context) error {
	if _, err := os.Stat(s.config.MigrationsPath); os.IsNotExist(err) {
		s.logger.Debug("migrations directory not found, skipping migrations",
			core.Field{"path", s.config.MigrationsPath})
		return nil
	}

	s.logger.Info("running database migrations",
		core.Field{"path", s.config.MigrationsPath})

	// Create migrations table if it doesn't exist
	createMigrationsTable := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`

	if _, err := s.db.ExecContext(ctx, createMigrationsTable); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Read migration files
	files, err := os.ReadDir(s.config.MigrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Apply migrations
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".sql" {
			continue
		}

		version := file.Name()

		// Check if migration already applied
		var count int
		checkQuery := "SELECT COUNT(*) FROM schema_migrations WHERE version = ?"
		if err := s.db.QueryRowContext(ctx, checkQuery, version).Scan(&count); err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if count > 0 {
			s.logger.Debug("migration already applied", core.Field{"version", version})
			continue
		}

		// Read and execute migration
		migrationPath := filepath.Join(s.config.MigrationsPath, file.Name())
		migrationSQL, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", migrationPath, err)
		}

		// Execute migration in transaction
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin migration transaction: %w", err)
		}

		if _, err := tx.ExecContext(ctx, string(migrationSQL)); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", version, err)
		}

		// Record migration
		recordQuery := "INSERT INTO schema_migrations (version) VALUES (?)"
		if _, err := tx.ExecContext(ctx, recordQuery, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", version, err)
		}

		s.logger.Info("migration applied successfully", core.Field{"version", version})
	}

	s.logger.Info("all migrations completed")
	return nil
}

// Utility methods for common database operations

// Exec executes a query without returning any rows
func (s *SQLite) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if !s.connected {
		return nil, fmt.Errorf("sqlite not connected")
	}
	return s.db.ExecContext(ctx, query, args...)
}

// Query executes a query that returns rows
func (s *SQLite) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if !s.connected {
		return nil, fmt.Errorf("sqlite not connected")
	}
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRow executes a query that is expected to return at most one row
func (s *SQLite) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// Transaction executes a function within a database transaction
func (s *SQLite) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	if !s.connected {
		return fmt.Errorf("sqlite not connected")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// GetStats returns database connection statistics
func (s *SQLite) GetStats() sql.DBStats {
	if s.db == nil {
		return sql.DBStats{}
	}
	return s.db.Stats()
}
