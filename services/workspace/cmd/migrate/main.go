package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/vasapolrittideah/flowspace-api/services/workspace/db/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
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
