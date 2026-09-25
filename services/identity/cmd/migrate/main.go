package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	sharedconfig "github.com/vasapolrittideah/flowspace-api/internal/config"
	"github.com/vasapolrittideah/flowspace-api/internal/logging"
	"github.com/vasapolrittideah/flowspace-api/services/identity/db/migrations"
)

type config struct {
	Environment string              `env:"ENVIRONMENT,required,notEmpty"`
	DatabaseURL sharedconfig.Secret `env:"DATABASE_URL,required,notEmpty"`
}

func main() {
	os.Exit(logging.Run(logging.New("identity-migrate", os.Getenv("ENVIRONMENT")), migrate))
}

func migrate() error {
	configuration, err := sharedconfig.Load[config]()
	if err != nil {
		return errors.New("invalid migration environment configuration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return run(ctx, string(configuration.DatabaseURL))
}

func run(ctx context.Context, databaseURL string) error {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer func() { _ = database.Close() }()
	goose.SetBaseFS(migrations.Files)
	if err := goose.SetDialect("postgres"); err != nil {
		return errors.New("invalid migration configuration")
	}
	if err := goose.UpContext(ctx, database, "."); err != nil {
		return errors.New("identity migration failed")
	}
	return nil
}
