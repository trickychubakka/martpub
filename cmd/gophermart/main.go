package main

import (
	"context"
	"fmt"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"log"
	"log/slog"
	"martnew/cmd/gophermart/initconf"
	"martnew/internal/compress"
	"martnew/internal/handlers"
	"martnew/internal/storage/inmem"
	"martnew/internal/storage/pgstorage"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Типа хранилища: true -- database, false -- inmem storage
var database = true

var logger *slog.Logger

func StorageInit(ctx context.Context, conf *initconf.Config, database bool) (handlers.Storager, error) {
	log.Println("StorageInit: conf is:", conf)

	if database {
		if conf.DatabaseDSN == "" {
			logger.Error("DatabaseDSN is not configured")
			return nil, fmt.Errorf("%s", "DatabaseDSN is not configured")
		}
		// Инициализация pgstorage
		conf.LogConf.Logger.Info("storeInit DatabaseDSN is configured, start to initialize pgstorage.")
		log.Println("StorageInit DatabaseDSN is configured, start to initialize pgstorage")
		log.Println("StorageInit: conf is", conf)
		store, err := pgstorage.New(ctx, conf)
		if err != nil {
			log.Println("storeInit error pgstorage initialization.")
			conf.LogConf.Logger.Error("storeInit error pgstorage initialization.")
			return nil, fmt.Errorf("%s %v", "storeInit error pgstorage initialization:", err)
		}
		return store, nil
	}

	store, err := inmem.New(ctx, conf)
	if err != nil {
		logger.Error("storeInit error inmem initialization.")
		return nil, fmt.Errorf("%s %v", "storeInit error inmem initialization:", err)
	}
	return &store, nil
}

func main() {

	var ctx, ctxRun context.Context
	var cancel, cancelRun context.CancelFunc
	var conf initconf.Config
	var store handlers.Storager
	// Create parent context
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	// создаем буферизованный канал для принятия задач в worker
	// константа workerPoolSize определена в tasks.go
	jobs := make(chan string, workerPoolSize)

	// Config initialization
	log.Println("main: Config initialization start")
	if err := initconf.InitConfig(&conf); err != nil {
		log.Fatal("Panic in initConfig")
	}
	logger = conf.LogConf.Logger

	// store initialization
	log.Println("main: Store initialization start")

	store, err := StorageInit(ctx, &conf, database)
	if err != nil {
		logger.Error("main: Storage DB type initialization error", "conf", conf)
		panic(err)
	}
	if database {
		logger.Info("Storage type is PGSTORAGE.")
		if _, ok := store.(pgstorage.PgStorage); !ok {
			panic("Error in type casting -- must be pgstorage.PgStorage")
		}
		defer store.(pgstorage.PgStorage).Close()
	} else {
		logger.Info("Storage type is INMEM.")
	}

	ctxRun, cancelRun = context.WithCancel(ctx)
	var wg sync.WaitGroup
	go task(ctxRun, &conf, store, &wg, jobs)

	// Остановка сервера
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancelRun()
		close(jobs)
		time.Sleep(10 * time.Second)
		logger.Warn("SERVER STOPPED.")
		os.Exit(1)
	}()

	// GIN init
	router := gin.Default()
	// set logger
	router.Use(handlers.UseLogger(ctx, &conf))
	router.Use(gzip.Gzip(gzip.DefaultCompression)) //-- standard GIN compress "github.com/gin-contrib/compress"

	router.Use(compress.GzipRequestHandle(ctx, &conf))
	router.POST("/api/user/register", handlers.RegisterHandler(ctx, &conf, store))
	router.POST("/api/user/login", handlers.LoginHandler(ctx, &conf, store))
	//router.Use(compress.GzipResponseHandle(compress.DefaultCompression))

	// Private handlers
	router.Use(handlers.PrivateCheck)
	router.GET("/api/user/balance", handlers.BalanceGetHandler(ctx, &conf, store))
	router.POST("/api/user/balance/withdraw", handlers.WithdrawHandler(ctx, &conf, store))
	router.GET("/api/user/orders", handlers.OrdersGetHandler(ctx, &conf, store))
	router.POST("/api/user/orders", handlers.OrdersPostHandler(ctx, &conf, store, jobs))
	router.GET("/api/user/withdrawals", handlers.WithdrawalsGetHandler(ctx, &conf, store))

	err = router.Run(conf.RunAddr)

	if err != nil {
		logger.Error("main: Panic in router.Run", "Error", err)
		panic(err)
	}
}
