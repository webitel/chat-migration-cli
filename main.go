package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/viper"
	"github.com/webitel/chat-migration-cli/internal/service"
	"github.com/webitel/chat-migration-cli/internal/store/newdb"
	"github.com/webitel/chat-migration-cli/internal/store/olddb"
)

// config holds all runtime configuration loaded from environment variables.
// All variables are prefixed with MIGRATION_ (e.g. MIGRATION_OLD_DB_DSN).
type config struct {
	OldDBDSN             string     // required: postgres DSN for the legacy chat DB
	NewDBDSN             string     // required: postgres DSN for the new microservices DB
	OldDBConns           int32      // OLD_DB_MAX_CONNS (default 5)
	NewDBConns           int32      // NEW_DB_MAX_CONNS (default 10)
	StartFrom            string     // START_FROM_STEP: skip steps before this one (optional)
	SingleStep           bool       // SINGLE_STEP: run only the step named by StartFrom, then stop (optional)
	LogLevel             slog.Level // LOG_LEVEL: debug|info|warn|error (default info)
	LogJSON              bool       // LOG_JSON: emit JSON instead of text (default false)
	EncryptionKey        string     // required: 32-byte AES-256 key for encrypting tokens
	Sync                 bool       // SYNC_MODE
	MigratePortalClients bool       // MIGRATE_PORTAL_CLIENTS
	BotMappingTable      string     // BOT_MAPPING_TABLE: schema.table; optional, empty disables pre-existing bot mapping
}

func main() {
	cfg := mustLoadConfig()

	log := buildLogger(cfg.LogLevel, cfg.LogJSON)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("connecting to databases")

	oldPool, err := buildPool(ctx, cfg.OldDBDSN, cfg.OldDBConns)
	if err != nil {
		log.Error("old DB connection failed", "error", err)
		os.Exit(1)
	}
	defer oldPool.Close()

	newPool, err := buildPool(ctx, cfg.NewDBDSN, cfg.NewDBConns)
	if err != nil {
		log.Error("new DB connection failed", "error", err)
		os.Exit(1)
	}
	defer newPool.Close()

	srcDB, err := olddb.New(oldPool, cfg.MigratePortalClients)
	if err != nil {
		log.Error("source DB init failed", "error", err)
		os.Exit(1)
	}

	dstDB, err := newdb.New(newPool, cfg.MigratePortalClients)
	if err != nil {
		log.Error("destination DB init failed", "error", err)
		os.Exit(1)
	}

	encryptor, err := service.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		log.Error("failed to create encryptor", "error", err)
		os.Exit(1)
	}

	converter := service.NewConverter(srcDB, dstDB, encryptor, cfg.Sync, cfg.MigratePortalClients, cfg.BotMappingTable)

	var runErr error
	switch {
	case cfg.SingleStep:
		log.Info("running single migration step", "step", cfg.StartFrom)
		runErr = converter.MigrateSingleStep(ctx, cfg.StartFrom)
	case cfg.StartFrom != "":
		log.Info("starting migration from step", "step", cfg.StartFrom)
		runErr = converter.MigrateFromStep(ctx, cfg.StartFrom)
	default:
		log.Info("starting full migration")
		runErr = converter.Migrate(ctx)
	}

	if runErr != nil {
		log.Error("migration failed", "error", runErr)
		os.Exit(1)
	}
	log.Info("migration completed")
}

func mustLoadConfig() config {
	v := viper.New()
	v.SetEnvPrefix("MIGRATION")
	v.AutomaticEnv()

	v.SetDefault("OLD_DB_MAX_CONNS", 5)
	v.SetDefault("NEW_DB_MAX_CONNS", 10)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_JSON", false)
	v.SetDefault("START_FROM_STEP", "")
	v.SetDefault("SINGLE_STEP", false)
	v.SetDefault("SYNC_MODE", false)
	v.SetDefault("MIGRATE_PORTAL_CLIENTS", false)
	v.SetDefault("BOT_MAPPING_TABLE", "")

	oldDSN := v.GetString("OLD_DB_DSN")
	newDSN := v.GetString("NEW_DB_DSN")
	encryptionKey := v.GetString("ENCRYPTION_KEY")
	if oldDSN == "" {
		slog.Error("MIGRATION_OLD_DB_DSN is required")
		os.Exit(1)
	}
	if newDSN == "" {
		slog.Error("MIGRATION_NEW_DB_DSN is required")
		os.Exit(1)
	}
	if encryptionKey == "" {
		slog.Error("MIGRATION_ENCRYPTION_KEY is required")
		os.Exit(1)
	}
	startFrom := v.GetString("START_FROM_STEP")
	singleStep := v.GetBool("SINGLE_STEP")
	if singleStep && startFrom == "" {
		slog.Error("MIGRATION_START_FROM_STEP is required when MIGRATION_SINGLE_STEP is enabled")
		os.Exit(1)
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(v.GetString("LOG_LEVEL"))); err != nil {
		level = slog.LevelInfo
	}

	botMappingTable := v.GetString("BOT_MAPPING_TABLE")
	if botMappingTable != "" {
		schema, table, ok := strings.Cut(botMappingTable, ".")
		if !ok || strings.TrimSpace(schema) == "" || strings.TrimSpace(table) == "" {
			slog.Error("MIGRATION_BOT_MAPPING_TABLE must be in \"schema.table\" format", "value", botMappingTable)
			os.Exit(1)
		}
	}
	return config{
		OldDBDSN:             oldDSN,
		NewDBDSN:             newDSN,
		OldDBConns:           int32(v.GetInt("OLD_DB_MAX_CONNS")),
		NewDBConns:           int32(v.GetInt("NEW_DB_MAX_CONNS")),
		StartFrom:            startFrom,
		SingleStep:           singleStep,
		LogLevel:             level,
		LogJSON:              v.GetBool("LOG_JSON"),
		EncryptionKey:        encryptionKey,
		Sync:                 v.GetBool("SYNC_MODE"),
		MigratePortalClients: v.GetBool("MIGRATE_PORTAL_CLIENTS"),
		BotMappingTable:      botMappingTable,
	}
}

func buildLogger(level slog.Level, json bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if json {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

func buildPool(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}
