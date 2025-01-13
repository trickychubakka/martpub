package database

import (
	"context"
	"database/sql"
	"log/slog"
)

// Database интерфейс для БД
type Database interface {
	Connect(string, *slog.Logger) error
	Close() error
	Exec(string, ...interface{}) (sql.Result, error)
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	Prepare(string) (*sql.Stmt, error)
	Query(string, ...interface{}) (*sql.Rows, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRow(string, ...interface{}) *sql.Row
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
	BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	Ping() error
}
