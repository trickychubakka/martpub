package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"io"
	"log"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/storage"
	"martnew/internal/storage/inmem"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

var conf initconf.Config
var err error
var confInitiated = false

// SetTestGinContext вспомогательная функция создания Gin контекста
func SetTestGinContext(w *httptest.ResponseRecorder, r *http.Request, user string) (*gin.Context, error) {
	// Во избежание повторной инициализации conf и logger при пакетном запуске тестов
	if !confInitiated {
		err = initconf.InitConfig(&conf)
		if err != nil {
			log.Println("SetTestGinContext error initconf.InitConfig. Error:", err)
		}
		log.Println("SetTestGinContext, conf is:", conf)

		logger, err = initconf.SetLogger(&conf)
		logger.Debug("SetTestGinContext, conf.LogConf.Logger is:", "conf.LogConf.Logger", conf.LogConf.Logger)
		if err != nil {
			log.Println("SetTestGinContext error initconf.SetLogger. Error:", err)
		}
		confInitiated = true
	}
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Request.Header.Set("Content-Type", "text/plain")
	c.Set("login", user)
	return c, nil
}

func StorageInitTest() Storager {

	user1 := storage.User{UserID: 1, Login: "user1", PwdHash: "hash1", Salt: "salt1", Balance: 100.0, Withdrawn: 100.0}
	user2 := storage.User{UserID: 2, Login: "user2", PwdHash: "e878b306fc0c2b3fd59bb1e311cc85bc13a7e43739e5089b0a2a1277d07fc582", Salt: "cc578f87", Balance: 200.0, Withdrawn: 200.0}
	user3 := storage.User{UserID: 3, Login: "user3", PwdHash: "hash3", Salt: "salt3", Balance: 300.0, Withdrawn: 300.0}
	user4 := storage.User{UserID: 4, Login: "user4", PwdHash: "hash4", Salt: "salt4", Balance: 400.0, Withdrawn: 0}
	user5 := storage.User{UserID: 5, Login: "user5", PwdHash: "hash5", Salt: "salt5", Balance: 0, Withdrawn: 0}

	order1 := storage.Order{OrderID: "1", UserID: 1, Status: "NEW", Accrual: 50, Withdrawal: 0, Date: "2024-12-24T01:00:00+03:00"}
	order2 := storage.Order{OrderID: "2", UserID: 1, Status: "NEW", Accrual: 150, Withdrawal: 0, Date: "2024-12-24T02:00:00+03:00"}
	order3 := storage.Order{OrderID: "3", UserID: 1, Status: "NEW", Accrual: 0, Withdrawal: 70, Date: "2024-12-24T03:00:00+03:00"}
	order9 := storage.Order{OrderID: "9", UserID: 1, Status: "NEW", Accrual: 0, Withdrawal: 30, Date: "2024-12-24T09:00:00+03:00"}

	order4 := storage.Order{OrderID: "4", UserID: 2, Status: "NEW", Accrual: 400, Withdrawal: 0, Date: "2024-12-24T04:00:00+03:00"}
	order5 := storage.Order{OrderID: "5", UserID: 2, Status: "NEW", Accrual: 0, Withdrawal: 200, Date: "2024-12-24T05:00:00+03:00"}

	order6 := storage.Order{OrderID: "6", UserID: 3, Status: "NEW", Accrual: 600, Withdrawal: 0, Date: "2024-12-24T06:00:00+03:00"}
	order7 := storage.Order{OrderID: "7", UserID: 3, Status: "NEW", Accrual: 0, Withdrawal: 300, Date: "2024-12-24T07:00:00+03:00"}

	order8 := storage.Order{OrderID: "8", UserID: 4, Status: "NEW", Accrual: 400, Withdrawal: 0, Date: "2024-12-24T08:00:00+03:00"}

	orders := [...]storage.Order{order1, order2, order3, order4, order5, order6, order7, order8, order9}
	users := [...]storage.User{user1, user2, user3, user4, user5}
	return &inmem.MemStorage{
		Orders: orders[:],
		Users:  users[:],
	}
}

func TestBalanceGetHandler(t *testing.T) {
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		balance     storage.BalanceResponse
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestBalanceGetHandler test ResponseCode 200",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/balance", nil),
				user: "user1",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json",
				balance:     storage.BalanceResponse{Balance: 100, Withdrawn: 100},
			},
		},
		{
			name: "TestBalanceGetHandler test ResponseCode 500",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/balance", nil),
				user: "user10",
			},
			want: want{
				code:        http.StatusInternalServerError,
				contentType: "application/json",
				//balance:     storage.BalanceResponse{Balance: 100, Withdrawn: 100},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			store := StorageInitTest()
			BalanceGetHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer
			var balance storage.BalanceResponse
			data, _ := io.ReadAll(tt.args.w.Body)
			if err := json.Unmarshal(data, &balance); err != nil {
				logger.Error("TestBalanceGetHandler error in json.Unmarshal", "Error:", err)
			}

			logger.Debug("TestBalanceGetHandler: response body", "balance", balance)
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
			if !reflect.DeepEqual(tt.want.balance, balance) {
				t.Errorf("BalanceGetHandler() = %v, want %v", balance, tt.want.balance)
			}
		})
	}
}

func TestLoginHandler(t *testing.T) {
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		signedResp  storage.SignedResponse
	}
	body, _ := json.Marshal(storage.UserLoginRequest{Login: "user2", Password: "password2"})
	wrongBody1 := []byte("WrongBody")
	bodyWithBadPassword, _ := json.Marshal(storage.UserLoginRequest{Login: "user2", Password: "password3"})
	bodyWithBadUser, _ := json.Marshal(storage.UserLoginRequest{Login: "user100", Password: "password3"})
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestLoginHandler test ResponseCode 200 StatusOK",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body)),
				user: "user2",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json; charset=utf-8",
				signedResp:  storage.SignedResponse{Token: "", Message: "logged in"},
			},
		},
		{
			name: "TestLoginHandler test ResponseCode 400 StatusBadRequest",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(wrongBody1)),
				user: "user2",
			},
			want: want{
				code: http.StatusBadRequest,
			},
		},
		{
			name: "TestLoginHandler test ResponseCode 401 StatusUnauthorized",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(bodyWithBadPassword)),
				user: "user2",
			},
			want: want{
				code: http.StatusUnauthorized,
			},
		},
		{
			name: "TestLoginHandler test ResponseCode 500 StatusInternalServerError",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(bodyWithBadUser)),
				user: "user2",
			},
			want: want{
				code: http.StatusInternalServerError,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			c.Request.Header.Set("Content-Type", "application/json")
			store := StorageInitTest()
			LoginHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer
			var signedResp storage.SignedResponse
			data, _ := io.ReadAll(tt.args.w.Body)
			if err := json.Unmarshal(data, &signedResp); err != nil {
				logger.Error("TestLoginHandler error in json.Unmarshal", "Error:", err)
			}

			logger.Debug("TestLoginHandler: response body", "signedResp", signedResp)
			logger.Debug("TestLoginHandler: response code", "ResponseCode:", res.Status())
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
			// Пробрасываем в want token, равный вычисленному в этот момент времени
			tt.want.signedResp.Token = signedResp.Token
			if !reflect.DeepEqual(tt.want.signedResp, signedResp) {
				t.Errorf("LoginHandler() = %v, want %v", signedResp, tt.want.signedResp)
			}
		})
	}
}

func TestOrdersGetHandler(t *testing.T) {
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		orders      []storage.Order
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestOrdersGetHandler test",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", nil),
				user: "user2",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json",
				// Из-за UserID int64 `json:"-"` в структуре User UserID в ответе будет нулевым,
				// так как по спецификации это поле не выдается в ответе
				orders: []storage.Order{{OrderID: "4", UserID: 0, Status: "NEW", Accrual: 400, Withdrawal: 0, Date: "2024-12-24T04:00:00+03:00"},
					{OrderID: "5", UserID: 0, Status: "NEW", Accrual: 0, Withdrawal: 200, Date: "2024-12-24T05:00:00+03:00"}},
			},
		},
		{
			name: "TestOrdersGetHandler test ResponseCode 204 StatusNoContent",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", nil),
				user: "user5",
			},
			want: want{
				code:        http.StatusNoContent,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestOrdersGetHandler test ResponseCode 500 StatusInternalServerError",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", nil),
				user: "user100",
			},
			want: want{
				code:        http.StatusInternalServerError,
				contentType: "text/plain; charset=utf-8",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			store := StorageInitTest()
			OrdersGetHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer
			logger.Debug("TestOrdersGetHandler: response body", "body", tt.args.w.Body)
			var orders []storage.Order
			data, _ := io.ReadAll(tt.args.w.Body)
			if err := json.Unmarshal(data, &orders); err != nil {
				logger.Error("TestOrdersGetHandler error in json.Unmarshal", "Error:", err)
			}
			logger.Debug("TestOrdersGetHandler: response body", "orders", orders)
			logger.Debug("TestOrdersGetHandler: response code", "ResponseCode:", res.Status())
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
			if !reflect.DeepEqual(tt.want.orders, orders) {
				t.Errorf("OrdersGetHandler() = %v, want %v", orders, tt.want.orders)
			}
		})
	}
}

func TestOrdersPostHandler(t *testing.T) {
	type args struct {
		w           *httptest.ResponseRecorder
		r           *http.Request
		ctx         context.Context
		conf        *initconf.Config
		store       Storager
		user        string
		contentType string
	}
	type want struct {
		code        int
		contentType string
		order       storage.Order
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestOrdersPostHandler test ResponseCode 202 StatusAccepted",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", bytes.NewReader([]byte("7777"))),
				user: "user4",
			},
			want: want{
				code:        http.StatusAccepted,
				contentType: "",
				order:       storage.Order{OrderID: "7777", UserID: 4, Status: "NEW", Accrual: 0, Withdrawal: 0, Date: ""},
			},
		},
		{
			name: "TestOrdersPostHandler test ResponseCode 200 StatusOK",
			args: args{
				w: httptest.NewRecorder(),
				// Уже существующий заказ, загруженный этим же пользователем
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", bytes.NewReader([]byte("8"))),
				user: "user4",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestOrdersPostHandler test ResponseCode 400 StatusBadRequest",
			args: args{
				w: httptest.NewRecorder(),
				// Уже существующий заказ, загруженный этим же пользователем
				r:           httptest.NewRequest(http.MethodGet, "/api/user/orders", bytes.NewReader([]byte("{8}"))),
				user:        "user4",
				contentType: "application/json",
			},
			want: want{
				code:        http.StatusBadRequest,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestOrdersPostHandler test ResponseCode 422 StatusUnprocessableEntity",
			args: args{
				w: httptest.NewRecorder(),
				// Уже существующий заказ, загруженный этим же пользователем
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", bytes.NewReader([]byte("Error"))),
				user: "user4",
			},
			want: want{
				code:        http.StatusUnprocessableEntity,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestOrdersPostHandler test ResponseCode 409 StatusConflict",
			args: args{
				w: httptest.NewRecorder(),
				// Уже существующий заказ, загруженный другим пользователем
				r:    httptest.NewRequest(http.MethodGet, "/api/user/orders", bytes.NewReader([]byte("1"))),
				user: "user4",
			},
			want: want{
				code:        http.StatusConflict,
				contentType: "text/plain; charset=utf-8",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			if tt.args.contentType != "" {
				c.Request.Header.Set("Content-Type", tt.args.contentType)
			}
			//logger.Info("TestBalanceGetHandler start")
			store := StorageInitTest()
			jobs := make(chan string, 10)
			OrdersPostHandler(tt.args.ctx, &conf, store, jobs)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer

			logger.Debug("TestOrdersPostHandler: response body", "body", tt.args.w.Body)

			if res.Status() == http.StatusAccepted {
				order, err := store.GetOrderByID(tt.args.ctx, &conf, "7777")
				if err == nil {
					tt.want.order.Date = order.Date
					if !reflect.DeepEqual(tt.want.order, *order) {
						t.Errorf("OrdersPostHandler() = %v, want %v", *order, tt.want.order)
					}
				}
			}
			logger.Debug("TestOrdersGetHandler: response code", "ResponseCode", res.Status())
			logger.Debug("TestOrdersGetHandler: Request", "ContentType", c.Request.Header.Get("Content-Type"))
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
		})
	}
}

func TestRegisterHandler(t *testing.T) {
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		user        storage.User
	}
	body, _ := json.Marshal(storage.UserLoginRequest{Login: "user6", Password: "password6"})
	bodyBadObject := []byte("Error")
	bodyUserConflict, _ := json.Marshal(storage.UserLoginRequest{Login: "user1", Password: "password1"})
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestRegisterHandler test ResponseCode 200 StatusOK",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(body)),
				user: "user6",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json; charset=utf-8",
			},
		},
		{
			name: "TestRegisterHandler test ResponseCode 400 StatusBadRequest",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(bodyBadObject)),
				user: "user4",
			},
			want: want{
				code:        http.StatusBadRequest,
				contentType: "",
			},
		},
		{
			name: "TestRegisterHandler test ResponseCode 409 StatusConflict",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewReader(bodyUserConflict)),
				user: "user4",
			},
			want: want{
				code:        http.StatusConflict,
				contentType: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			c.Request.Header.Set("Content-Type", "application/json")
			store := StorageInitTest()
			RegisterHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer

			logger.Debug("TestRegisterHandler: response code", "ResponseCode", res.Status())
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
			// Проверяем, появился ли в хранилище новый пользователь user6
			//logger.Debug("TestRegisterHandler: ", "store", store)
			if res.Status() == http.StatusOK {
				if _, err := store.GetUser(tt.args.ctx, &conf, storage.UserLoginRequest{Login: "user6", Password: "password6"}); err != nil {
					t.Errorf("GetUser() error = %v", err)
				}
			}
		})
	}
}

func TestWithdrawHandler(t *testing.T) {
	// Начальные условия до теста: User{UserID: 4, Login: "user4", PwdHash: "hash4", Salt: "salt4", Balance: 400.0, Withdrawn: 0}
	// Order с номером 7777 отсутствует в списке заказов
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		order       storage.Order
		user        storage.User
	}
	// Используемые для теста объекты:
	// order := storage.Order{OrderID: 7777, UserID: 4, Status: "NEW", Accrual: 0, Withdrawal: 100, Date: ""}
	// withdraw := storage.WithdrawRequest{OrderID: "7777", Withdraw: 100}
	body, _ := json.Marshal(storage.WithdrawRequest{OrderID: "7777", Withdraw: 100})
	bodyNotEnoughPoints, _ := json.Marshal(storage.WithdrawRequest{OrderID: "7777", Withdraw: 10000})
	bodyWrongOrderID, _ := json.Marshal(storage.WithdrawRequest{OrderID: "Error", Withdraw: 100})

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestWithdrawHandler test ResponseCode 200 StatusOK",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/balance/withdraw", bytes.NewReader(body)),
				user: "user4",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json",
				order:       storage.Order{OrderID: "7777", UserID: 4, Status: "NEW", Accrual: 0, Withdrawal: 100, Date: ""},
				user:        storage.User{UserID: 4, Login: "user4", PwdHash: "hash4", Salt: "salt4", Balance: 300.0, Withdrawn: 100},
			},
		},
		{
			name: "TestWithdrawHandler test ResponseCode 402 StatusPaymentRequired",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/balance/withdraw", bytes.NewReader(bodyNotEnoughPoints)),
				user: "user4",
			},
			want: want{
				code:        http.StatusPaymentRequired,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestWithdrawHandler test ResponseCode 422 StatusUnprocessableEntity",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/balance/withdraw", bytes.NewReader(bodyWrongOrderID)),
				user: "user4",
			},
			want: want{
				code:        http.StatusUnprocessableEntity,
				contentType: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			c.Request.Header.Set("Content-Type", "application/json")
			store := StorageInitTest()
			WithdrawHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer
			logger.Debug("TestWithdrawHandler: response body", "body", tt.args.w.Body)

			orderTest, _ := store.GetOrderByID(tt.args.ctx, &conf, "7777")
			if res.Status() == http.StatusOK {
				tt.want.order.Date = orderTest.Date
				if !reflect.DeepEqual(tt.want.order, *orderTest) {
					t.Errorf("WithdrawHandler() = %v, want %v", *orderTest, tt.want.order)
				}
				// Проверка соответствия баланса пользователя ожидаемому
				userTest, err := store.GetUser(tt.args.ctx, &conf, storage.UserLoginRequest{Login: "user4", Password: "password4"})
				if err != nil {
					t.Errorf("TestWithdrawHandler: GetUser() error = %v", err)
				}
				if !reflect.DeepEqual(tt.want.user, *userTest) {
					t.Errorf("WithdrawHandler() = %v, want %v", *userTest, tt.want.user)
				}
			}
			logger.Debug("TestWithdrawHandler: ", "order", orderTest)
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
		})
	}
}

func TestWithdrawalsGetHandler(t *testing.T) {
	type args struct {
		w     *httptest.ResponseRecorder
		r     *http.Request
		ctx   context.Context
		conf  *initconf.Config
		store Storager
		user  string
	}
	type want struct {
		code        int
		contentType string
		withdrawals []storage.Withdrawal
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "Positive TestWithdrawalsGetHandler test ResponseCode 200 StatusOK",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil),
				user: "user1",
			},
			want: want{
				code:        http.StatusOK,
				contentType: "application/json",
				// Используемые для теста объекты:
				// order3 := storage.Order{OrderID: 3, UserID: 1, Status: "NEW", Accrual: 0, Withdrawal: 70, Date: "2024-12-24T03:00:00+03:00"}
				// order9 := storage.Order{OrderID: 9, UserID: 1, Status: "NEW", Accrual: 0, Withdrawal: 30, Date: "2024-12-24T09:00:00+03:00"}
				withdrawals: []storage.Withdrawal{{OrderID: "3", Withdrawal: 70, Date: ""}, {OrderID: "9", Withdrawal: 30, Date: ""}},
			},
		},
		{
			name: "TestWithdrawalsGetHandler test ResponseCode 204 StatusNoContent",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil),
				user: "user5",
			},
			want: want{
				code:        http.StatusNoContent,
				contentType: "text/plain; charset=utf-8",
			},
		},
		{
			name: "TestWithdrawalsGetHandler test ResponseCode 500 StatusInternalServerError",
			args: args{
				w:    httptest.NewRecorder(),
				r:    httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil),
				user: "user1888",
			},
			want: want{
				code:        http.StatusInternalServerError,
				contentType: "text/plain; charset=utf-8",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := SetTestGinContext(tt.args.w, tt.args.r, tt.args.user)
			if err != nil {
				t.Fatal(err)
			}
			c.Request.Header.Set("Content-Type", "application/json")
			store := StorageInitTest()
			WithdrawalsGetHandler(tt.args.ctx, &conf, store)(c)
			defer tt.args.w.Result().Body.Close()
			res := c.Writer
			var withdrawals []storage.Withdrawal
			data, _ := io.ReadAll(tt.args.w.Body)
			if err := json.Unmarshal(data, &withdrawals); err != nil {
				logger.Error("TestWithdrawalsGetHandler error in json.Unmarshal", "Error:", err)
			}

			logger.Debug("TestWithdrawalsGetHandler: response body", "withdrawals", withdrawals)
			for i, w := range withdrawals {
				tt.want.withdrawals[i].Date = w.Date
			}
			logger.Debug("TestRegisterHandler: response code", "ResponseCode", res.Status())
			assert.Equal(t, tt.want.code, res.Status())
			assert.Equal(t, tt.want.contentType, res.Header().Get("Content-Type"))
			if !reflect.DeepEqual(tt.want.withdrawals, withdrawals) {
				t.Errorf("WithdrawalsGetHandler() = %v, want %v", withdrawals, tt.want.withdrawals)
			}
		})
	}
}
