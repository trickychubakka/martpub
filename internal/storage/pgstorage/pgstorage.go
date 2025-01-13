// Package pgstorage -- Postgres реализация хранилища
package pgstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/database"
	"martnew/internal/storage"
)

var logger *slog.Logger

// UserLoginMapping -- map[Login]UserID for GetUserID acceleration. Key is Login, value is UserID
type UserLoginMapping map[string]int64

type PgStorage struct {
	pgDB *database.Postgresql
}

// randomString генерация случайной строки заданной длины для поля salt
func randomString(length int) string {
	return uuid.NewString()[:length]
}

func New(ctx context.Context, conf *initconf.Config) (PgStorage, error) {
	// logger initialization for this package
	logger = conf.LogConf.Logger

	pg := database.Postgresql{}
	logger.Info("Connecting to database ...", "pg", pg)
	err := pg.Connect(conf.DatabaseDSN, conf.LogConf.Logger)
	if err != nil {
		logger.Error("New: pg.Connect error", "Error", err)
		logger.Error("New: pg.Connect error", "Conf", conf)
		//log.Fatal("Error creating table users :", err)
		return PgStorage{&pg}, err
	}

	logger.Info("Check or create users table")
	sqlQuery := `create table IF NOT EXISTS users
(
    user_id   SERIAL       not null
        constraint users_pk
            primary key,
    login varchar(30) not null UNIQUE,
    pwd_hash  varchar(100) not null,
    pwd_salt  varchar(10)  not null,
    balance   DECIMAL(20, 4) default 0 check(balance >= 0),
    withdrawn DECIMAL(20, 4) default 0 check(withdrawn >= 0)
)`
	err = database.PgExecWrapper(pg.ExecContext, ctx, sqlQuery)
	if err != nil {
		logger.Error("Error creating table users", "Error", err)
		return PgStorage{&pg}, err
	}

	logger.Info("Check or create orders table")
	sqlQuery = `create table IF NOT EXISTS orders
(
    order_id  varchar(40)      not null
        constraint orders_pk
            primary key,
    user_id SERIAL      not null
        constraint orders_users_user_id_fk
            references users,
    status  varchar(10) not null,
    accrual DECIMAL(20, 4) default 0 check(accrual >= 0),
    withdrawal DECIMAL(20, 4) default 0 check(withdrawal >= 0),
    date    varchar(30) not null
)`
	err = database.PgExecWrapper(pg.ExecContext, ctx, sqlQuery)
	if err != nil {
		logger.Error("Error creating table orders", "Error", err)
		return PgStorage{&pg}, err
	}
	return PgStorage{&pg}, nil
}

func (pg PgStorage) RegisterUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error) {
	logger.Info("Register user")
	u := storage.User{}
	// Генерируем случайный salt для защиты hash пароля пользователя
	salt := randomString(8)
	// вычисляем hash строки password+salt, далее кодируем в HEX для сохранения в базу
	hash := sha256.Sum256([]byte(loginReq.Password + salt))
	u.Login = loginReq.Login
	u.PwdHash = hex.EncodeToString(hash[:])
	u.Salt = salt
	sqlQuery := `INSERT INTO users(login, pwd_hash, pwd_salt, balance, withdrawn) VALUES ($1, $2, $3, $4, $5)`
	err := database.PgExecWrapper(pg.pgDB.ExecContext, ctx, sqlQuery, u.Login, u.PwdHash, u.Salt, 0, 0)
	if err != nil {
		logger.Error("error PG register user", "Error", err)
		return nil, err
	}
	return &u, nil
}

func (pg PgStorage) GetUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error) {
	logger.Info("GetUser", "login", loginReq.Login)
	u := storage.User{}
	sqlQuery := "SELECT * FROM users WHERE login = $1"
	row := database.PgQueryRowWrapper(pg.pgDB.QueryRowContext, ctx, sqlQuery, loginReq.Login)
	if err := row.Scan(&u.UserID, &u.Login, &u.PwdHash, &u.Salt, &u.Balance, &u.Withdrawn); err != nil {
		logger.Error("GetUser error", "Error", err)
		return nil, fmt.Errorf("%s %v", "GetUser error:", err)
	}
	logger.Debug("GetUser: User is:", "User", u)
	return &u, nil
}

func (pg PgStorage) GetUserID(ctx context.Context, conf *initconf.Config, login string) (int64, error) {
	logger.Info("GetUserID ", "login", login)
	var userID int64
	//// Соответствие не найдено, ищем в базе и обновляем кэш после успешного поиска
	sqlQuery := "SELECT user_id FROM users WHERE login = $1"
	row := database.PgQueryRowWrapper(pg.pgDB.QueryRowContext, ctx, sqlQuery, login)
	if err := row.Scan(&userID); err != nil {
		logger.Error("GetUserID error:", "Error", err)
		return -1, fmt.Errorf("%s %v", "GetUserID error:", err)
	}
	logger.Debug("GetUser: User is", "userID", userID)
	return userID, nil
}

// UpdateUserBalance -- изменение баланса пользователя данными из Order
func (pg PgStorage) UpdateUserBalance(ctx context.Context, conf *initconf.Config, order *storage.Order) error {
	logger.Info("UpdateUser with Order:", "Order", order)
	sqlQuery := "UPDATE users SET balance = balance + $2 - $3, withdrawn = withdrawn + $3 WHERE user_id = $1"
	err := database.PgExecWrapper(pg.pgDB.ExecContext, ctx, sqlQuery, order.UserID, order.Accrual, order.Withdrawal)
	if err != nil {
		logger.Error("UpdateUserBalance error", "Error", err)
		return err
	}
	return nil
}

func (pg PgStorage) RegisterOrder(ctx context.Context, conf *initconf.Config, o *storage.Order) error {
	logger.Info("Register order", "order", o.OrderID)
	// старт транзакции -- в случае проблем на UpdateUserBalance необходимо откатить первую операцию
	tx, err := pg.pgDB.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("RegisterOrder: error BeginTx", "Error", err)
		return err
	}

	sqlQuery := `INSERT INTO orders(order_id, user_id, status, accrual, withdrawal, date) VALUES ($1, $2, $3, $4, $5, $6)`
	err = database.PgExecWrapper(tx.ExecContext, ctx, sqlQuery, o.OrderID, o.UserID, "NEW", 0, o.Withdrawal, o.Date)
	if err != nil {
		logger.Error("RegisterOrder: error register order", "Error", err)
		return err
	}
	// Вносим изменения в баланс user-а.
	// Необходимо для изменения withdrawn - balance меняется через внешнюю Accrual систему и метод UpdateOrderByAccrual
	if o.Accrual > 0 || o.Withdrawal > 0 {
		err = pg.UpdateUserBalance(ctx, conf, o)
		if err != nil {
			logger.Warn("RegisterOrder: UpdateUserBalance error, start ROLLBACK")
			err1 := tx.Rollback()
			if err1 != nil {
				logger.Error("RegisterOrder: error rollback", "Error", err1)
				return err1
			}
			logger.Error("RegisterOrder: UpdateUserBalance error", "Error", err)
			return err
		}
	}
	return tx.Commit()
}

func (pg PgStorage) UpdateOrderByAccrual(ctx context.Context, conf *initconf.Config, orderID string, status string, accrual float32) error {
	logger.Info("Update order", "orderID", orderID)
	tx, err := pg.pgDB.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("UpdateOrderByAccrual: error BeginTx", "Error", err)
		return err
	}

	if accrual == 0 {
		sqlQuery := `UPDATE orders SET status = $1 WHERE order_id = $2`
		err = database.PgExecWrapper(tx.ExecContext, ctx, sqlQuery, status, orderID)
		if err != nil {
			logger.Error("UpdateOrderByAccrual: error register order", "Error", err)
			return err
		}
		return tx.Commit()
	}

	sqlQuery1 := `UPDATE orders SET status = $1, accrual = $2 WHERE order_id = $3`
	err = database.PgExecWrapper(tx.ExecContext, ctx, sqlQuery1, status, accrual, orderID)
	if err != nil {
		logger.Error("UpdateOrderByAccrual: error update order", "Error", err)
		return err
	}
	sqlQuery2 := `UPDATE users SET balance = balance + $1 WHERE user_id = (select user_id from orders where order_id = $2)`
	err = database.PgExecWrapper(tx.ExecContext, ctx, sqlQuery2, accrual, orderID)
	if err != nil {
		err1 := tx.Rollback()
		logger.Error("UpdateOrderByAccrual: error update order", "Error", err1)
		return err1
	}
	return tx.Commit()
}

func (pg PgStorage) GetOrderByID(ctx context.Context, conf *initconf.Config, orderID string) (*storage.Order, error) {
	logger.Info("GetOrderByID", "Order", orderID)
	o := storage.Order{}
	sqlQuery := `SELECT * FROM orders WHERE order_id = $1`
	row := database.PgQueryRowWrapper(pg.pgDB.QueryRowContext, ctx, sqlQuery, orderID)
	if err := row.Scan(&o.OrderID, &o.UserID, &o.Status, &o.Accrual, &o.Withdrawal, &o.Date); err != nil {
		logger.Error("GetOrderByID error:", "Error", err)
		return &o, fmt.Errorf("%s %v", "GetOrderByID error:", err)
	}
	logger.Info("GetOrderByID: Order is:", "Order", o)
	return &o, nil
}

func (pg PgStorage) GetOrdersByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Order, error) {
	logger.Info("GetOrders for user ", "User", user)
	var orders []storage.Order

	userID, err := pg.GetUserID(ctx, conf, user)
	if err != nil {
		logger.Error("GetOrdersByUser error in GetUserID:", "Error", err)
		return nil, err
	}

	sqlQuery := "SELECT order_id, status, accrual, withdrawal, date FROM orders where user_id = $1 order by date desc"
	rows, err := database.PgQueryWrapper(pg.pgDB.QueryContext, ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var order storage.Order

		err = rows.Scan(&order.OrderID, &order.Status, &order.Accrual, &order.Withdrawal, &order.Date)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &orders, nil
}

func (pg PgStorage) GetBalanceByUser(ctx context.Context, conf *initconf.Config, user string) (*storage.BalanceResponse, error) {
	logger.Info("GetBalanceByUser", "User", user)
	balance := storage.BalanceResponse{}
	sqlQuery := `SELECT balance, withdrawn FROM users WHERE login = $1`
	row := database.PgQueryRowWrapper(pg.pgDB.QueryRowContext, ctx, sqlQuery, user)
	if err := row.Scan(&balance.Balance, &balance.Withdrawn); err != nil {
		logger.Error("GetBalanceByUser error:", "Error", err)
		return nil, err
	}
	logger.Debug("GetOrderByID: BalanceResponse is:", "Balance", balance)
	return &balance, nil
}

func (pg PgStorage) GetWithdrawalsByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Withdrawal, error) {
	logger.Info("GetWithdrawalsByUser", "User", user)
	var withdrawals []storage.Withdrawal

	userID, err := pg.GetUserID(ctx, conf, user)
	if err != nil {
		logger.Error("GetWithdrawalsByUser error in GetUserID:", "Error", err)
		return nil, err
	}

	sqlQuery := "SELECT order_id, withdrawal, date FROM orders where user_id = $1 and withdrawal > 0 order by date desc"
	rows, err := database.PgQueryWrapper(pg.pgDB.QueryContext, ctx, sqlQuery, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var withdrawal storage.Withdrawal

		err = rows.Scan(&withdrawal.OrderID, &withdrawal.Withdrawal, &withdrawal.Date)
		if err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, withdrawal)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &withdrawals, nil
}

func (pg PgStorage) RegisterWithdraw(ctx context.Context, conf *initconf.Config, o *storage.Order) error {
	logger.Info("WithdrawReg start for withdraw")
	// старт транзакции -- в случае проблем на UpdateUserBalance необходимо откатить первую операцию
	tx, err := pg.pgDB.BeginTx(ctx, nil)
	if err != nil {
		logger.Error("RegisterWithdraw: error BeginTx", "Error", err)
		return err
	}

	sqlQuery := `INSERT INTO orders(order_id, user_id, status, accrual, withdrawal, date) VALUES ($1, $2, $3, $4, $5, $6)`
	err = database.PgExecWrapper(tx.ExecContext, ctx, sqlQuery, o.OrderID, o.UserID, o.Status, 0, o.Withdrawal, o.Date)
	if err != nil {
		logger.Error("RegisterWithdraw: error register order", "Error", err)
		return err
	}
	// Вносим изменения в баланс user-а
	err = pg.UpdateUserBalance(ctx, conf, o)
	if err != nil {
		logger.Warn("RegisterOrder: UpdateUserBalance error, start ROLLBACK")
		err1 := tx.Rollback()
		if err1 != nil {
			logger.Error("RegisterOrder: error rollback", "Error", err1)
			return err1
		}
		logger.Error("RegisterOrder: UpdateUserBalance error", "Error", err)
		return err
	}
	//return nil
	return tx.Commit()
}

func (pg PgStorage) Close() error {
	return pg.pgDB.Close()
}
