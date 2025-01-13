// Package handlers -- реализация Gin хендлеров
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/jackc/pgx/v5/pgconn"
	luhnmod10 "github.com/luhnmod10/go"
	"io"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/storage"
	"net/http"
	"strings"
	"time"
)

// Проверять ли входящий OrderID по алгоритму Луна
const (
	checkLuhn = true
	//checkLuhn = false
)

var logger *slog.Logger

var jwtSecretKey = []byte("very-secret-key")

type Storager interface {
	RegisterUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error)
	GetUser(ctx context.Context, conf *initconf.Config, loginReq storage.UserLoginRequest) (*storage.User, error)
	GetUserID(ctx context.Context, conf *initconf.Config, login string) (int64, error)
	UpdateUserBalance(ctx context.Context, conf *initconf.Config, order *storage.Order) error
	RegisterOrder(ctx context.Context, conf *initconf.Config, order *storage.Order) error
	GetOrderByID(ctx context.Context, conf *initconf.Config, orderID string) (*storage.Order, error)
	GetOrdersByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Order, error)
	GetBalanceByUser(ctx context.Context, conf *initconf.Config, user string) (*storage.BalanceResponse, error)
	GetWithdrawalsByUser(ctx context.Context, conf *initconf.Config, user string) (*[]storage.Withdrawal, error)
	RegisterWithdraw(ctx context.Context, conf *initconf.Config, order *storage.Order) error
	UpdateOrderByAccrual(ctx context.Context, conf *initconf.Config, orderID string, status string, accrual float32) error
}

func parseToken(jwtToken string) (*jwt.Token, error) {
	token, err := jwt.Parse(jwtToken, func(token *jwt.Token) (interface{}, error) {
		if _, OK := token.Method.(*jwt.SigningMethodHMAC); !OK {
			return nil, errors.New("bad signed method received")
		}
		return jwtSecretKey, nil
	})

	if err != nil {
		return nil, errors.New("bad jwt token")
	}

	if !token.Valid {
		fmt.Println("Token is not valid")
		return nil, errors.New("bad jwt token")
	}

	return token, nil
}

func extractBearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("zero Authorization header value given")
	}

	jwtToken := strings.Split(header, " ")
	if len(jwtToken) != 2 {
		return "", errors.New("incorrectly formatted authorization header")
	}

	return jwtToken[1], nil
}

func jwtPayloadFromRequest(tokenString string) (jwt.MapClaims, error) {
	logger.Info("jwtPayloadFromRequest: token is:", "Token", tokenString)

	jwtToken, err := parseToken(tokenString)
	logger.Info("jwtPayloadFromRequest: jwtToken is:", "jwtToken", jwtToken)

	if err != nil {
		logger.Error("jwtPayloadFromRequest error:", "Error", err)
		return nil, err
	}

	payload, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		logger.Error("jwtPayloadFromRequest error: jwtPayloadFromRequest token is not *jwt.MapClaims")
		return nil, errors.New("jwtPayloadFromRequest token is not *jwt.MapClaims")
	}
	return payload, nil
}

func PrivateCheck(c *gin.Context) {
	logger.Info("PrivateCheck start")
	jwtToken, err := extractBearerToken(c.GetHeader("Authorization"))
	if err != nil {
		logger.Error("PrivateCheck error:", "Error", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, storage.UnsignedResponse{
			Message: err.Error(),
		})
		return
	}

	payload, err := jwtPayloadFromRequest(jwtToken)
	if err != nil {
		logger.Error("PrivateCheck error:", "Error", err)
		c.AbortWithStatusJSON(http.StatusUnauthorized, storage.UnsignedResponse{
			Message: err.Error(),
		})
		return
	}
	logger.Debug("privateCheck: jwt payload is:", "Payload", payload, "User", payload["sub"])
	logger.Debug("PrivateCheck successful")
	c.Set("login", payload["sub"].(string))
	c.Next()
}

// RegisterHandler -- Gin handler, регистрация пользователя
func RegisterHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("Register handler start")
		logger.Debug("RegisterHandler: Request Header is:", "Header", c.Request.Header)
		var user *storage.User
		loginRequest := storage.UserLoginRequest{}

		// Content-Type check
		if c.GetHeader("Content-Type") != "application/json" {
			logger.Error("RegisterHandler: wrong Content-Type")
			http.Error(c.Writer, "RegisterHandler: wrong Content-Type", http.StatusBadRequest)
			return
		}

		jsn, err := io.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("RegisterHandler: Error in io.ReadAll")
			http.Error(c.Writer, "RegisterHandler: Error in json body read", http.StatusBadRequest)
			return
		}

		err = json.Unmarshal(jsn, &loginRequest)
		if err != nil {
			logger.Error("RegisterHandler: Error in json.Unmarshal", "Error", err, "jsn", string(jsn))
			c.Status(http.StatusBadRequest)
			return
		}

		user, err = store.RegisterUser(ctx, conf, loginRequest)
		// for inmem
		if err != nil && err.Error() == "trying to register duplicate login" {
			c.Status(http.StatusConflict)
			logger.Error("RegisterHandler: Error in RegisterUser", "Error", err, "jsn", string(jsn))
			return
		}
		// for PG
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			logger.Debug("pgErr.Code is:", "pgErr.Code", pgErr.Code)
			switch pgErr.Code {
			case "23505": // duplicate login error PG
				c.Status(http.StatusConflict)
			default:
				c.Status(http.StatusInternalServerError)
			}
			logger.Error("RegisterHandler: Error in RegisterUser", "Error", err, "jsn", string(jsn))
			return
		}

		token, err := user.AuthUser(ctx, loginRequest, jwtSecretKey, conf.LogConf.Logger)
		if err != nil {
			logger.Error("LoginHandler: error in AuthUser function")
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Header("Authorization", "Bearer "+token)
		c.JSON(http.StatusOK, storage.SignedResponse{
			Token:   token,
			Message: "logged in",
		})
	}
}

// LoginHandler -- Gin handler, аутентификация пользователя
func LoginHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {

		logger.Info("Login handler start")
		logger.Debug("LoginHandler: Request Header is:", "Header", c.Request.Header)

		// Content-Type check
		if c.GetHeader("Content-Type") != "application/json" {
			logger.Error("RegisterHandler: wrong Content-Type")
			http.Error(c.Writer, "RegisterHandler: wrong Content-Type", http.StatusBadRequest)
			return
		}

		var user *storage.User
		loginRequest := storage.UserLoginRequest{}

		jsn, err := io.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("RegisterHandler: Error in io.ReadAll", "Error", err)
			http.Error(c.Writer, "LoginHandler: Error in json body read", http.StatusBadRequest)
			return
		}

		err = json.Unmarshal(jsn, &loginRequest)
		if err != nil {
			logger.Error("LoginHandler: Error in json.Unmarshal", "Error", err, "jsn", string(jsn))
			c.Status(http.StatusBadRequest)
			return
		}

		user, err = store.GetUser(ctx, conf, loginRequest)
		if err != nil {
			logger.Error("LoginHandler: Error in GetUser", "Error", err, "jsn", string(jsn))
			c.Status(http.StatusInternalServerError)
			return
		}
		//logger.Debug("Login handler: User is:", "User", user)
		token, err := user.AuthUser(ctx, loginRequest, jwtSecretKey, conf.LogConf.Logger)
		if err != nil {
			switch errors.Is(err, storage.ErrBadCredentials) {
			case true:
				logger.Error("LoginHandler: auth failed")
				c.Status(http.StatusUnauthorized)
			case false:
				logger.Error("LoginHandler: error in AuthUser function")
				c.Status(http.StatusInternalServerError)
			}
			return
		}
		c.Header("Authorization", "Bearer "+token)
		c.JSON(http.StatusOK, storage.SignedResponse{
			Token:   token,
			Message: "logged in",
		})
	}
}

// OrdersPostHandler -- Gin handler, загрузка пользователем номера заказа для расчёта
func OrdersPostHandler(ctx context.Context, conf *initconf.Config, store Storager, jobs chan<- string) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("Orders post start")
		logger.Debug("OrdersPostHandler: Request Header is:", "Header", c.Request.Header)
		logger.Debug("OrdersPostHandler. Store is", "Store", store)

		// неверный формат запроса, code 400
		if c.GetHeader("Content-Type") != "text/plain" {
			logger.Error("OrdersPostHandler: wrong Content-Type")
			http.Error(c.Writer, "OrdersPostHandler: wrong Content-Type", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			// неверный формат запроса, code 400
			logger.Error("OrdersPostHandler: Error in json body read", "Error", err)
			http.Error(c.Writer, "OrdersPostHandler: Error in json body read", http.StatusBadRequest)
			return
		}

		order := &storage.Order{}

		order.OrderID = string(body)
		if checkLuhn && !luhnmod10.Valid(order.OrderID) {
			logger.Error("OrdersPostHandler: wrong order_id -- luhnmod10.Valid error", "Error", err, "strOrderID", order.OrderID)
			http.Error(c.Writer, "OrdersPostHandler: wrong order_id format", http.StatusUnprocessableEntity)
			return
		}

		login := c.Keys["login"].(string)
		logger.Debug("OrdersPostHandler: unpacked orderNumber:", "Order", order.OrderID, "User", c.Keys["login"])

		order.UserID, err = store.GetUserID(ctx, conf, login)
		if err != nil {
			logger.Error("OrdersPostHandler: Error in GetUserID", "Error", err)
			http.Error(c.Writer, "OrdersPostHandler: Error in GetUserID", http.StatusInternalServerError)
			return
		}
		order.Date = time.Now().Format(time.RFC3339)
		order.Status = "NEW"

		err = store.RegisterOrder(ctx, conf, order)
		if err != nil {
			logger.Error("OrdersPostHandler: Error in RegisterOrder:", "Error", err)
			// Если заказ с таким order_id уже существует
			if err.Error() == "ERROR: duplicate key value violates unique constraint \"orders_pk\" (SQLSTATE 23505)" ||
				err.Error() == "RegisterOrder error - duplicate OrderID" {
				logger.Error("ERROR duplicate order_id", "Error", err)
				tmpOrder, err1 := store.GetOrderByID(ctx, conf, order.OrderID)
				if err1 != nil {
					logger.Error("OrdersPostHandler: Error in GetOrderByID", "Error", err1)
					http.Error(c.Writer, "OrdersPostHandler: Error in GetOrderByID", http.StatusInternalServerError)
				}
				switch tmpOrder.UserID {
				case order.UserID:
					// уже есть заказ с таким order_id от этого пользователя, code 200
					logger.Error("OrdersPostHandler: there is order with the same order_id from this user")
					http.Error(c.Writer, "OrdersPostHandler: there is order with the same order_id from this user", http.StatusOK)
					return
				default:
					// уже есть заказ с таким order_id от другого пользователя, code 409
					logger.Error("OrdersPostHandler: there is order with this order_id from other user")
					http.Error(c.Writer, "OrdersPostHandler: there is order with this order_id from other user", http.StatusConflict)
					return
				}
			}
			http.Error(c.Writer, "OrdersPostHandler: Error in RegisterOrder", http.StatusInternalServerError)
			return
		}

		// Отсылаем в main worker номер заказа
		jobs <- order.OrderID

		logger.Debug("OrdersPostHandler: order accepted for processing", "Order", order.OrderID)
		c.Status(http.StatusAccepted)
		//return
	}
}

// OrdersGetHandler -- Gin handler, получение списка загруженных пользователем номеров заказов,
// статусов их обработки и информации о начислениях
func OrdersGetHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("Orders get start")

		login := c.Keys["login"].(string)
		logger.Debug("OrdersGetHandler: Requested orders list for user", "User", login)

		orders, err := store.GetOrdersByUser(ctx, conf, login)
		if err != nil {
			logger.Error("OrdersGetHandler: Error in GetOrdersByUser", "Error", err, "User", login)
			http.Error(c.Writer, "OrdersGetHandler: No orders found for user", http.StatusInternalServerError)
			return
		}
		// Если для пользователя не найдено заказов
		if len(*orders) == 0 {
			logger.Warn("OrdersGetHandler: No orders found for user", "User", login)
			http.Error(c.Writer, "OrdersGetHandler: No orders found for user", http.StatusNoContent)
			return
		}
		c.Header("content-type", "application/json")
		c.IndentedJSON(http.StatusOK, orders)
	}
}

// BalanceGetHandler -- Gin handlers получения текущего баланса счета баллов лояльности
func BalanceGetHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("Balance get start")
		login := c.Keys["login"].(string)
		conf.LogConf.Logger.Debug("BalanceGetHandler check logger")

		balance, err := store.GetBalanceByUser(ctx, conf, login)
		if err != nil {
			logger.Error("Balance GetHandler: Error in GetBalanceByUser for user", "User", login, "Error", err)
			http.Error(c.Writer, "Balance GetHandler: Error in GetBalanceByUser for login", http.StatusInternalServerError)
		}
		c.Header("content-type", "application/json")
		c.IndentedJSON(http.StatusOK, balance)
	}
}

// WithdrawHandler -- Gin handler, запрос на списание баллов с накопительного счёта в счёт оплаты нового заказа
func WithdrawHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("Withdraw request start")

		// Content-Type check
		if c.GetHeader("Content-Type") != "application/json" {
			logger.Error("WithdrawHandler: wrong Content-Type")
			http.Error(c.Writer, "WithdrawHandler: wrong Content-Type", http.StatusBadRequest)
			return
		}

		login := c.Keys["login"].(string)
		withdraw := storage.WithdrawRequest{}

		jsn, err := io.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("WithdrawHandler: Error in json body read", "Error", err)
			http.Error(c.Writer, "WithdrawHandler: Error in json body read", http.StatusBadRequest)
			return
		}

		err = json.Unmarshal(jsn, &withdraw)
		if err != nil {
			logger.Error("WithdrawHandler: Error in json.Unmarshal", "Error", err, "jsn", string(jsn))
			http.Error(c.Writer, "WithdrawHandler: Error in json body read", http.StatusBadRequest)
			return
		}

		if checkLuhn && !luhnmod10.Valid(withdraw.OrderID) {
			logger.Error("WithdrawHandler: wrong order_id -- luhnmod10.Valid error", "Error", err, "strOrderID", withdraw.OrderID)
			http.Error(c.Writer, "WithdrawHandler: wrong order_id format", http.StatusUnprocessableEntity)
			return
		}

		userID, err := store.GetUserID(ctx, conf, login)
		if err != nil {
			logger.Error("WithdrawHandler: Error in GetUserID", "Error", err, "jsn", string(jsn))
			http.Error(c.Writer, "WithdrawHandler: Error in GetUserID", http.StatusInternalServerError)
			return
		}

		order := storage.Order{
			OrderID:    withdraw.OrderID,
			UserID:     userID,
			Status:     "REGISTERED",
			Accrual:    0,
			Withdrawal: withdraw.Withdraw,
			Date:       time.Now().Format(time.RFC3339),
		}

		err = store.RegisterOrder(ctx, conf, &order)
		if err != nil {
			if err.Error() == "not enough points" || err.Error() == "ERROR: new row for relation \"users\" violates check constraint \"users_balance_check\" (SQLSTATE 23514)" {
				logger.Error("WithdrawHandler: not enough points")
				http.Error(c.Writer, "WithdrawHandler: not enough points", http.StatusPaymentRequired)
				return
			}
			logger.Error("WithdrawHandler: Error in RegisterWithdraw", "Error", err.Error())
			http.Error(c.Writer, "WithdrawHandler: Error in RegisterWithdraw", http.StatusInternalServerError)
			return
		}

		c.Header("content-type", "application/json")
		c.Status(http.StatusOK)
	}
}

// WithdrawalsGetHandler -- Gin handler, получение информации о выводе средств с накопительного счёта пользователем
func WithdrawalsGetHandler(ctx context.Context, conf *initconf.Config, store Storager) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger.Info("WithdrawalsGetHandler get start")
		login := c.Keys["login"].(string)

		withdrawals, err := store.GetWithdrawalsByUser(ctx, conf, login)
		if err != nil {
			logger.Error("WithdrawalsGetHandler: Error in GetOrdersByUser", "Error", err, "User", login)
			http.Error(c.Writer, "WithdrawalsGetHandler: No withdrawals found for user", http.StatusInternalServerError)
			return
		}

		logger.Info("WithdrawalsGetHandler: withdrawals", "Withdrawals", withdrawals)

		// Если для пользователя не найдено заказов
		if len(*withdrawals) == 0 {
			logger.Warn("WithdrawalsGetHandler: No withdrawals found for user", "User", login)
			http.Error(c.Writer, "WithdrawalsGetHandler: No withdrawals found for user", http.StatusNoContent)
			return
		}

		c.Header("content-type", "application/json")
		c.IndentedJSON(http.StatusOK, withdrawals)
	}
}

// UseLogger -- middleware проброса объекта logger в хэндлеры и остальные методы пакета
func UseLogger(_ context.Context, conf *initconf.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		logger = conf.LogConf.Logger
		logger.Debug("Set logger", "logger", logger)
	}
}
