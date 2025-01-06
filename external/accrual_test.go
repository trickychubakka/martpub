package external

import (
	"context"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/storage"
	"martnew/internal/storage/inmem"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

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

func TestRegisterAccrual(t *testing.T) {
	type args struct {
		ctx          context.Context
		orderNumber  string
		conf         *initconf.Config
		jsonResponse string
	}
	tests := []struct {
		name    string
		args    args
		want    *AccrualResponse
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "Positive RegisterAccrual Test",
			args: args{
				ctx:         context.Background(),
				orderNumber: "777",
				conf: &initconf.Config{
					AccrualRunAddr: "http://localhost",
					LogConf: initconf.LoggingConf{
						Logger:   logger,
						LocalRun: true,
						LogLevel: slog.LevelDebug,
					},
				},
				jsonResponse: `{"order": "777", "status": "REGISTERED", "accrual": 0}`,
			},
			want: &AccrualResponse{
				Order:   "777",
				Status:  "REGISTERED",
				Accrual: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, _ = initconf.SetLogger(tt.args.conf)
			store := StorageInitTest()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != ("/api/orders/" + tt.args.orderNumber) {
					t.Errorf("Expected to request '/api/orders/%s, got: %s", tt.args.orderNumber, r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "text/plain" {
					t.Errorf("Expected Content-Type: text/plain header, got: %s", r.Header.Get("Content-Type"))
				}

				body := []byte(tt.args.jsonResponse)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(body)
			}))
			defer server.Close()

			// устанавливаем в conf динамический адрес тестового Accrual MOCK сервера
			tt.args.conf.AccrualRunAddr = server.URL

			got, err := RegisterAccrual(tt.args.ctx, tt.args.conf, store, tt.args.orderNumber)
			logger.Debug("TestRegisterAccrual: AccrualResponse is", "AccrualResponse", got)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterAccrual() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RegisterAccrual() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSendRequest(t *testing.T) {
	type args struct {
		client      *http.Client
		url         string
		contentType string
		conf        *initconf.Config
	}
	tests := []struct {
		name    string
		args    args
		want    *http.Response
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "Positive SendRequest Test",
			args: args{
				client: &http.Client{},
				url:    "http://localhost" + accrualURL + "777",
				conf: &initconf.Config{
					AccrualRunAddr: "http://localhost",
					LogConf: initconf.LoggingConf{
						Logger:   logger,
						LocalRun: true,
						LogLevel: slog.LevelDebug,
					},
				},
				contentType: "text/plain",
			},
			want:    &http.Response{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			logger, _ = initconf.SetLogger(tt.args.conf)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

				if r.Header.Get("Content-Type") != "text/plain" {
					t.Errorf("Expected Content-Type: text/plain header, got: %s", r.Header.Get("Content-Type"))
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			tt.args.conf.AccrualRunAddr = server.URL

			response, err := SendRequest(tt.args.client, tt.args.conf.AccrualRunAddr, tt.args.contentType, tt.args.conf)
			if err != nil {
				logger.Error("TestSendRequest error", "error", err)
				t.Errorf("SendRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			defer response.Body.Close()
			if (err != nil) != tt.wantErr {
				logger.Error("TestSendRequest() error = %v, wantErr %v", "Error", err)
				t.Errorf("SendRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
