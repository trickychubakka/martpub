package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/storage"
	"net/http"
	"time"
)

// Accrual system URL
const accrualURL = "/api/orders/"

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

// Набор из 3-х таймаутов для повтора операции в случае retriable-ошибки
var timeoutsRetryConst = [3]int{1, 3, 5}

var logger *slog.Logger

var client = &http.Client{}

type AccrualResponse struct {
	Order   string  `json:"order"`
	Status  string  `json:"status"`
	Accrual float32 `json:"accrual,omitempty"`
}

// SendRequest функция отсылки запроса на адрес внешнего Accrual сервиса
func SendRequest(client *http.Client, url string, contentType string, conf *initconf.Config) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		logger.Error("SendRequest. Error creating request", "Error", err)
		return nil, fmt.Errorf("%s %v", "SendRequest: http.NewRequest error.", err)
	}

	req.Header.Set("Content-Type", contentType)

	// Отсылка сформированного запроса req. Если сервер не отвечает -- работа агента завершается
	response, err := client.Do(req)

	if err != nil {
		logger.Warn("SendRequest error in 1 attempt.", "Error", err)
		for i, t := range timeoutsRetryConst {
			logger.Warn("SendRequest. Trying to recover after ", "seconds", t, "attempt number", i+1)
			time.Sleep(time.Duration(t) * time.Second)
			response, err = client.Do(req)
			if err != nil {
				logger.Warn("SendRequest: attempt ", "attempt", i+1, "Error", err)
				if i == 2 {
					logger.Error("SendRequest: client.Do error", "Error", err)
					return nil, fmt.Errorf("%s %v", "SendRequest: client.Do error", err)
				}
				continue
			}
			return response, nil
		}
	}
	if response != nil {
		logger.Debug("SendRequest: response is", "Response", response)

	}
	return response, nil
}

// RegisterAccrual функция регистрации и обновления статуса запроса во внешнюю Accrual систему
func RegisterAccrual(ctx context.Context, conf *initconf.Config, store Storager, orderNumber string) (*AccrualResponse, error) {
	logger = conf.LogConf.Logger
	logger.Debug("RegisterAccrual started for", "Order", orderNumber)

	reqURL := conf.AccrualRunAddr + accrualURL + orderNumber
	logger.Debug("RegisterAccrual. reqURL", "reqURL", reqURL)

	response, err := SendRequest(client, reqURL, "text/plain", conf)
	if err != nil {
		logger.Error("RegisterAccrual. Error in SendRequest", "Error", err)
		return nil, err
	}
	defer response.Body.Close()

	logger.Debug("RegisterAccrual response status", "Status", response.Status)

	// при получении 204 http.StatusNoContent
	if response.StatusCode == http.StatusNoContent {
		logger.Info("RegisterAccrual: StatusNoContent")
		return nil, fmt.Errorf("StatusNoContent")
	}
	// при получении 429 StatusTooManyRequests
	if response.StatusCode == http.StatusTooManyRequests {
		logger.Info("RegisterAccrual: StatusTooManyRequests", "Retry-After", response.Header.Get("Retry-After"))
		timeout := response.Header.Get("Retry-After") + "s"
		return nil, fmt.Errorf("StatusTooManyRequests %s", timeout)
	}

	if response.StatusCode == http.StatusInternalServerError {
		logger.Info("RegisterAccrual: StatusInternalServerError")
		return nil, fmt.Errorf("StatusInternalServerError")
	}

	if response.Header.Get("Content-Type") != "application/json" {
		logger.Error("RegisterAccrual. Wrong Content-Type error", "Content", response.Header.Get("Content-Type"))
		return nil, fmt.Errorf("RegisterAccrual. Wrong Content-Type error")
	}

	jsn, err := io.ReadAll(response.Body)
	if err != nil {
		logger.Error("RegisterAccrual: Error in io.ReadAll", "Error", err)
		return nil, err
	}

	accrualResp := AccrualResponse{}

	err = json.Unmarshal(jsn, &accrualResp)
	if err != nil {
		logger.Error("RegisterAccrual: Error in json.Unmarshal", "Error", err, "jsn", string(jsn))
		return nil, err
	}

	logger.Info("UpdateOrderByAccrual start with", "AccrualResponse", accrualResp)

	logger.Debug("RegisterAccrual: AccrualResponse object", "AccrualResponse", accrualResp, "Order", orderNumber)
	return &accrualResp, nil
}
