package main

import (
	"fmt"
	"io/fs"
	"log/slog"

	dbembed "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/logger"
)

func migrationsFS() fs.FS {
	sub, err := fs.Sub(dbembed.MigrationsFS, "migrations")
	if err != nil {
		panic(fmt.Sprintf("embedded migrations: %v", err))
	}
	return sub
}

func runMigrate(args []string) error {
	cfg, err := provideConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger.Init(cfg.Log.Level, cfg.Log.Format)
	log := logger.L

	migrateCmd := args[0]
	var migrateArgs []string
	if len(args) > 1 {
		migrateArgs = args[1:]
	}

	if err := db.RunMigrate(log, cfg.Postgres, migrationsFS(), migrateCmd, migrateArgs); err != nil {
		log.Error("migration failed", slog.Any("error", err))
		return err
	}
	return nil
}
