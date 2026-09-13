package main

import (
	"context"
	"database/sql"
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
	Environment string `env:"ENVIRONMENT,required,notEmpty"`
}

func main() {
	os.Exit(logging.Run(logging.New("workspace-migrate", os.Getenv("ENVIRONMENT")), migrate))
}

func migrate() error {
	if _, err := sharedconfig.Load[config](); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return run(ctx)
}

func run(ctx context.Context) error {
	database, err := sql.Open("pgx", os.Getenv("DATABASE_URL"))
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
