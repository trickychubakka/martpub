package database

import (
	"context"
	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log/slog"
	"martnew/conf"
	"regexp"
	"strings"
)

// Переменная для объекта logger-а
var logger *slog.Logger

type Postgresql struct {
	Cfg *conf.Config
	db  *sql.DB
}

func (p *Postgresql) Connect(connStr string, logr *slog.Logger) error {
	p.Cfg = &conf.Config{}
	logger = logr
	zp := regexp.MustCompile(`(://)|/|@|:|\?`)
	connStrMap := zp.Split(connStr, -1)
	// Получаем map вида [postgres user password address port user sslmode=disable]
	logger.Info("Connection string", "connStrMap", connStrMap, "len", len(connStrMap))
	p.Cfg.Database.User = connStrMap[1]
	p.Cfg.Database.Password = connStrMap[2]
	p.Cfg.Database.Host = connStrMap[3]
	// Если в connstr содержится port -- длина connStrMap 7
	// Если в connstr НЕ содержится port -- длина connStrMap 6
	if len(connStrMap) == 7 {
		p.Cfg.Database.Dbname = connStrMap[5]
		p.Cfg.Database.Sslmode = strings.Split(connStrMap[6], "=")[1]
		// Если в connstr содержится port -- длина connStrMap 7
	} else if len(connStrMap) == 6 {
		p.Cfg.Database.Dbname = connStrMap[4]
		p.Cfg.Database.Sslmode = strings.Split(connStrMap[5], "=")[1]
	}

	logger.Info("Connect: Database config", "p.Cfg.Database", p.Cfg.Database)
	logger.Info("Connect: Connection string to database", "connStr", connStr)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		logger.Error("Connect: Error connecting to database", "Error", err)
		return err
	}
	logger.Info("Connect: Connected to database with DSN ", "connStr", connStr, "with db", db)
	p.db = db
	return nil
}

func (p *Postgresql) Close() error {
	return p.db.Close()
}

func (p *Postgresql) Exec(query string, args ...interface{}) (sql.Result, error) {
	return p.db.Exec(query, args...)
}

func (p *Postgresql) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}

func (p *Postgresql) Prepare(query string) (*sql.Stmt, error) {
	return p.db.Prepare(query)
}

func (p *Postgresql) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.Query(query, args...)
}

func (p *Postgresql) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}

func (p *Postgresql) QueryRow(query string, args ...interface{}) *sql.Row {
	return p.db.QueryRow(query, args...)
}

func (p *Postgresql) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}

func (p *Postgresql) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return p.db.BeginTx(ctx, opts)
}

func (p *Postgresql) Ping() error {
	return p.db.Ping()
}
