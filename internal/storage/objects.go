// Package storage - набор общих объектов и их методов, используемых в различных реализациях хранилища
package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"log/slog"
	"time"
)

var (
	ErrBadCredentials = errors.New("login or password is incorrect")
)

type UnsignedResponse struct {
	Message interface{} `json:"message"`
}

type SignedResponse struct {
	Token   string `json:"token"`
	Message string `json:"message"`
}

type UserLoginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type User struct {
	UserID    int64   `json:"user_id"`
	Login     string  `json:"login"`
	PwdHash   string  `json:"pwd_hash"`
	Salt      string  `json:"salt"`
	Balance   float32 `json:"balance"`
	Withdrawn float32 `json:"withdrawn"`
}

func (u *User) AuthUser(ctx context.Context, loginReq UserLoginRequest, jwtSecretKey []byte, logger *slog.Logger) (string, error) {
	logger.Info("AuthUser", "Login", loginReq.Login)
	hash0, err := hex.DecodeString(u.PwdHash)
	if err != nil {
		logger.Error("AuthUser error: hex.DecodeString error", "Error", err)
		return "", err
	}
	hash := sha256.Sum256([]byte(loginReq.Password + u.Salt))

	if !bytes.Equal(hash0, hash[:]) {
		logger.Error("AuthUser: auth failed for user", "Login", loginReq.Login)
		return "", ErrBadCredentials
	}
	logger.Info("AuthUser: auth successful", "Login", loginReq.Login)

	payload := jwt.MapClaims{
		"sub": loginReq.Login,
		"exp": time.Now().Add(time.Hour * 72).Unix(),
	}

	// Создаем новый JWT-токен и подписываем его по алгоритму HS256
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	tokenString, err := token.SignedString(jwtSecretKey)
	if err != nil {
		logger.Error("AuthUser error: token.SignedString error", "Error", err)
		return "", err
	}

	return tokenString, nil
}

type BalanceResponse struct {
	Balance   float32 `json:"current"`
	Withdrawn float32 `json:"withdrawn"`
}

type Withdrawal struct {
	OrderID    string  `json:"order"`
	Withdrawal float32 `json:"sum"`
	Date       string  `json:"processed_at"`
}

type WithdrawRequest struct {
	OrderID  string  `json:"order"`
	Withdraw float32 `json:"sum"`
}

type Order struct {
	OrderID    string  `json:"number"`
	UserID     int64   `json:"-"`
	Status     string  `json:"status"`
	Accrual    float32 `json:"accrual,omitempty"`
	Withdrawal float32 `json:"withdrawal,omitempty"`
	Date       string  `json:"uploaded_at"`
}
