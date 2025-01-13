package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"log"
	"time"
)

// Набор из 3-х таймаутов для повтора операции в случае retriable-ошибки
var timeoutsRetryConst = [3]int{1, 3, 5}

// pgErrorRetriable функция определения принадлежности PostgreSQL ошибки к классу retriable.
func pgErrorRetriable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		logger.Error("pgErrorRetriable: PostgreSQL error", "pgErr.Message", pgErr.Message, "pgErr.Code", pgErr.Code)
		if pgerrcode.IsConnectionException(pgErr.Code) {
			logger.Error("PostgreSQL error : IsConnectionException is true.")
			return true
		}
	}
	return false
}

// PgExecWrapper -- wrapper для запросов типа ExecContext
func PgExecWrapper(f func(ctx context.Context, query string, args ...any) (sql.Result, error), ctx context.Context, sqlQuery string, args ...any) error {
	_, err := f(ctx, sqlQuery, args...)
	// Если ошибка retriable
	if pgErrorRetriable(err) {
		for i, t := range timeoutsRetryConst {
			logger.Warn("PgExecWrapper, RetriableError: Trying to recover after", "seconds", t, "attempt number", i+1)
			time.Sleep(time.Duration(t) * time.Second)
			_, err = f(ctx, sqlQuery, args...)
			if err != nil {
				if i == 2 {
					log.Println("PgExecWrapper RetriableError: error in wrapped function", "Error", err)
					return err
				}
				continue
			}
			logger.Warn("PgExecWrapper RetriableError, attempt success", "attempt", i+1)
			return nil
		}
	}
	// Если ошибка non-retriable
	if err != nil {
		logger.Error("PgExecWrapper Non-RetriableError: Panic in wrapped function", "Error", err)
		return err
	}
	// Если ошибки нет
	return nil
}

// PgQueryRowWrapper -- wrapper для SQL запросов типа QueryRowContext
func PgQueryRowWrapper(f func(ctx context.Context, query string, args ...any) *sql.Row, ctx context.Context, sqlQuery string, args ...any) *sql.Row {
	row := f(ctx, sqlQuery, args...)
	// Если ошибка retriable
	if pgErrorRetriable(row.Err()) {
		for i, t := range timeoutsRetryConst {
			logger.Warn("PgQueryRowWrapper, RetriableError: Trying to recover after ", "seconds", t, "attempt number", i+1)
			time.Sleep(time.Duration(t) * time.Second)
			row = f(ctx, sqlQuery, args...)
			if row.Err() != nil {
				if i == 2 {
					logger.Error("PgQueryRowWrapper RetriableError: Panic in wrapped function:", "Error", row.Err())
				}
				continue
			}
			logger.Warn("PgQueryRowWrapper RetriableError: success attempt", "attempt", i+1)
			return row
		}
	}
	// Если ошибка non-retriable
	if row.Err() != nil {
		logger.Error("PgQueryRowWrapper Non-RetriableError: Panic in wrapped function", "Error", row.Err())
	}
	// Если ошибки нет
	return row
}

// PgQueryWrapper -- wrapper для SQL запросов типа QueryContext
func PgQueryWrapper(f func(ctx context.Context, query string, args ...any) (*sql.Rows, error), ctx context.Context, sqlQuery string, args ...any) (*sql.Rows, error) {
	rows, err := f(ctx, sqlQuery, args...)
	// Если ошибка retriable
	if pgErrorRetriable(err) {
		for i, t := range timeoutsRetryConst {
			logger.Warn("PgQueryWrapper, RetriableError: Trying to recover after ", "seconds", t, "attempt number", i+1)
			time.Sleep(time.Duration(t) * time.Second)
			rows, err = f(ctx, sqlQuery, args...)
			if err != nil {
				if i == 2 {
					logger.Error("pgQueryWrapper RetriableError: Panic in wrapped function:", "Error", err)
					return nil, fmt.Errorf("%s %v", "pgQueryWrapper RetriableError: Panic in wrapped function:", err)
				}
				continue
			}
			logger.Warn("PgQueryWrapper RetriableError: success attempt", "attempt", i+1)
			return rows, nil
		}
	}
	// Если ошибка non-retriable
	if err != nil {
		logger.Error("PgQueryWrapper Non-RetriableError: Panic in wrapped function", "Error", err)
		return nil, fmt.Errorf("%s %v", "PgQueryWrapper Non-RetriableError: Panic in wrapped function:", err)
	}
	// Если ошибки нет
	return rows, nil
}
