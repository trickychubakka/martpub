// Package inmem -- реализация in memory хранилища
package inmem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/storage"
)

var logger *slog.Logger

const RFC3339local = "2006-01-02T15:04:05Z"

type MemStorage struct {
	Orders []storage.Order
	Users  []storage.User
}

// randomString генерация случайной строки заданной длины для поля salt
func randomString(length int) string {
	return uuid.NewString()[:length]
}

// New -- создание нового inMem хранилища
func New(ctx context.Context, conf *initconf.Config) (MemStorage, error) {
	// logger initialization for this package
	logger = conf.LogConf.Logger

	return MemStorage{
		Orders: make([]storage.Order, 0),
		Users:  make([]storage.User, 0),
	}, nil
}

func (m *MemStorage) RegisterUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error) {
	logger = conf.LogConf.Logger
	logger.Info("Register user")
	u := storage.User{}
	// Генерируем случайный salt для защиты hash пароля пользователя
	salt := randomString(8)
	// вычисляем hash строки password+salt, далее кодируем в HEX для сохранения в базу
	hash := sha256.Sum256([]byte(loginReq.Password + salt))
	l := len(m.Users)
	if l == 0 {
		u.UserID = 1
	} else {
		u.UserID = m.Users[l-1].UserID + 1
	}
	for _, tmpUser := range m.Users {
		if tmpUser.Login == loginReq.Login {
			logger.Error("Error, trying to register duplicate login")
			return nil, fmt.Errorf("trying to register duplicate login")
		}
	}
	u.Login = loginReq.Login
	u.PwdHash = hex.EncodeToString(hash[:])
	u.Salt = salt
	m.Users = append(m.Users, u)

	logger.Debug("RegisterUser: INMEM storage is", "store", m)
	return &u, nil
}

func (m *MemStorage) GetUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetUser", "login", loginReq.Login)

	for _, tmpUser := range m.Users {
		if tmpUser.Login == loginReq.Login {
			logger.Debug("GetUser: User is:", "User", tmpUser)
			return &tmpUser, nil
		}
	}
	logger.Error("GetUser error for login", "Login", loginReq.Login)
	return nil, fmt.Errorf("GetUser error for login")
}

func (m *MemStorage) GetUserID(ctx context.Context, conf *initconf.Config, login string) (int64, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetUserID ", "login", login)

	for _, tmpUser := range m.Users {
		if tmpUser.Login == login {
			logger.Debug("GetUserID: UserID is:", "UserID", tmpUser.UserID)
			logger.Debug("GetUserID: INMEM storage is", "store", m)
			return tmpUser.UserID, nil
		}
	}
	logger.Debug("GetUserID error")
	return -1, fmt.Errorf("%s", "GetUserID error")
}

// UpdateUserBalance -- изменение баланса пользователя данными из Order
func (m *MemStorage) UpdateUserBalance(ctx context.Context, conf *initconf.Config, order *storage.Order) error {
	logger.Info("UpdateUser with Order:", "Order", order)

	for i, tmpUser := range m.Users {
		if tmpUser.UserID == order.UserID {
			tmpBalance := m.Users[i].Balance
			tmpBalance = tmpBalance + order.Accrual - order.Withdrawal
			if tmpBalance < 0 {
				logger.Error("UpdateUserBalance error - not enough points")
				return fmt.Errorf("not enough points")
			} else {
				m.Users[i].Balance = tmpBalance
			}
			m.Users[i].Withdrawn = m.Users[i].Withdrawn + order.Withdrawal
			logger.Debug("UpdateUserBalance: INMEM storage is", "store", m)
			return nil
		}
	}
	logger.Error("UpdateUserBalance error")
	return fmt.Errorf("%s", "UpdateUserBalance error")
}

func (m *MemStorage) RegisterOrder(ctx context.Context, conf *initconf.Config, o *storage.Order) error {
	logger.Info("Register order", "order", o.OrderID)

	for _, tmpOrder := range m.Orders {
		if tmpOrder.OrderID == o.OrderID {
			logger.Error("RegisterOrder error - duplicate OrderID")
			return fmt.Errorf("%s", "RegisterOrder error - duplicate OrderID")
		}
	}
	// Если в новом заказе поля Accrual или Withdrawal не нулевые -- изменяем баланс пользователя
	if o.Accrual > 0 || o.Withdrawal > 0 {
		err := m.UpdateUserBalance(ctx, conf, o)
		if err != nil {
			return err
		}
	}
	// Если UpdateUserBalance не выдал ошибку -- добавляем новый Order
	m.Orders = append(m.Orders, *o)

	logger.Debug("RegisterOrder: INMEM storage is", "store", m)
	return nil
}

func (m *MemStorage) UpdateOrderByAccrual(ctx context.Context, conf *initconf.Config, orderID string, status string, accrual float32) error {
	logger.Info("Update order", "orderID", orderID)
	var orderIndex int
	var userID int64

	for i, tmpOrder := range m.Orders {
		if tmpOrder.OrderID == orderID {
			m.Orders[i].Status = status
			orderIndex = i
			userID = tmpOrder.UserID
		}
	}
	// Если Accrual в AccrualResponse равен 0 -- выходим
	if accrual == 0 {
		return nil
	}
	// Если Accrual в AccrualResponse не 0 -- меняем Accrual в Order и меняем баланс пользователя
	m.Orders[orderIndex].Accrual = accrual

	for i, tmpUser := range m.Users {
		if tmpUser.UserID == userID {
			m.Users[i].Balance = m.Users[i].Balance + accrual
		}
	}
	return nil
}

func (m *MemStorage) GetOrderByID(ctx context.Context, conf *initconf.Config, orderID string) (*storage.Order, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetOrderByID", "Order", orderID)

	for _, tmpOrder := range m.Orders {
		if tmpOrder.OrderID == orderID {
			logger.Info("GetOrderByID: Order is:", "Order", tmpOrder)
			logger.Debug("GetOrderByID: INMEM storage is", "store", m)
			return &tmpOrder, nil
		}
	}
	logger.Error("GetOrderByID: error")
	return nil, fmt.Errorf("%s", "GetOrderByID error:")
}

func (m *MemStorage) GetOrdersByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Order, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetOrders for user ", "User", user)
	//orders := []storage.Order{}
	var orders []storage.Order

	userID, err := m.GetUserID(ctx, conf, user)
	if err != nil {
		logger.Error("GetOrdersByUser error in GetUserID", "Error", err)
		return nil, err
	}
	logger.Debug("GetOrdersByUser: ", "UserID", userID, "Login", user)

	for _, tmpOrder := range m.Orders {
		if tmpOrder.UserID == userID {
			orders = append(orders, tmpOrder)
		}
	}

	logger.Debug("GetOrdersByUser:", "orders", orders)
	return &orders, nil
}

func (m *MemStorage) GetBalanceByUser(ctx context.Context, conf *initconf.Config, user string) (*storage.BalanceResponse, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetBalanceByUser", "User", user)
	balance := storage.BalanceResponse{}

	for _, tmpUser := range m.Users {
		if tmpUser.Login == user {
			balance.Balance = tmpUser.Balance
			balance.Withdrawn = tmpUser.Withdrawn
			logger.Debug("GetBalanceByUser: INMEM storage is", "store", m)
			return &balance, nil
		}
	}
	logger.Error("GetBalanceByUser error")
	return nil, fmt.Errorf("%s", "GetBalanceByUser error")
}

func (m *MemStorage) GetWithdrawalsByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Withdrawal, error) {
	logger = conf.LogConf.Logger
	logger.Info("GetWithdrawalsByUser", "User", user)
	var withdrawals []storage.Withdrawal

	userID, err := m.GetUserID(ctx, conf, user)
	if err != nil {
		logger.Error("GetWithdrawalsByUser error in GetUserID", "Error", err)
		return nil, err
	}

	for _, o := range m.Orders {
		if o.UserID == userID && o.Withdrawal > 0 {
			withdrawals = append(withdrawals, storage.Withdrawal{OrderID: o.OrderID, Withdrawal: o.Withdrawal, Date: o.Date})
		}
	}

	logger.Debug("GetWithdrawalsByUser:", "withdrawals", withdrawals)
	logger.Debug("GetWithdrawalsByUser: INMEM storage is", "store", m)
	return &withdrawals, nil
}

func (m *MemStorage) RegisterWithdraw(ctx context.Context, conf *initconf.Config, o *storage.Order) error {
	logger.Info("WithdrawReg start for withdraw")

	err := m.RegisterOrder(ctx, conf, o)
	if err != nil {
		logger.Error("RegisterWithdraw: error register order", "Error", err)
		return err
	}

	err = m.UpdateUserBalance(ctx, conf, o)
	if err != nil {
		return err
	}

	logger.Debug("RegisterWithdraw: INMEM storage is", "store", m)
	return nil
}
