package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
)

type config struct {
	Environment string              `env:"ENVIRONMENT,required,notEmpty"`
	DatabaseURL sharedconfig.Secret `env:"DATABASE_URL"`
}

func main() {
	os.Exit(logging.Run(logging.New("workspace-migrate", os.Getenv("ENVIRONMENT")), migrate))
}

func migrate() error {
	configuration, err := sharedconfig.Load[config]()
	if err != nil {
		return err
	}
	if configuration.DatabaseURL == "" {
		for _, key := range []string{"PGHOST", "PGDATABASE", "PGUSER", "PGPASSWORD"} {
			if os.Getenv(key) == "" {
				return errors.New("DATABASE_URL or PGHOST, PGDATABASE, PGUSER, and PGPASSWORD are required")
			}
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return run(ctx, string(configuration.DatabaseURL))
}

func run(ctx context.Context, databaseURL string) error {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("configure database: %w", err)
	}
	defer func() { _ = database.Close() }()

	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("configure migrations: %w", err)
	}
	if err := goose.UpContext(ctx, database, "."); err != nil {
		return fmt.Errorf("migrate workspace database: %w", err)
	}
	return nil
}
