package main

import (
	"context"
	"martnew/cmd/gophermart/initconf"
	"martnew/external"
	"martnew/internal/handlers"
	"strings"
	"sync"
	"time"
)

const (
	workerPoolSize = 5
)

// worker горутина выполнения запросов ко внешней Accrual системе
func worker(ctx context.Context, conf *initconf.Config, store handlers.Storager, workerID int, wg *sync.WaitGroup, jobs <-chan string, results chan<- string) error {
	logger = conf.LogConf.Logger
	logger.Debug("worker: start worker", "workerID", workerID)
	accrual := &external.AccrualResponse{Status: ""}
	var err error
	tmpStatus := ""
	defer func() {
		logger.Debug("worker: finished worker", "wg.Done()", workerID)
		wg.Done()
	}()

	for orderID := range jobs {
		logger.Debug("worker: start worker for accrual", "number", orderID)
		// цикл до получения окончательных статусов PROCESSED или INVALID
		for accrual.Status != "PROCESSED" && accrual.Status != "INVALID" {
			select {
			// проверяем не завершён ли ещё контекст и выходим, если завершён
			case <-ctx.Done():
				logger.Debug("Worker stopped with ctxDone", "worker", workerID)
				return nil

			// выполняем нужный нам код
			default:
				logger.Debug("worker: RegisterAccrual run with", "OrderID", orderID, "workerID", workerID)
				accrual, err = external.RegisterAccrual(ctx, conf, store, orderID)
				if err != nil {
					switch err.Error() {
					case "StatusInternalServerError":
						logger.Error("worker got StatusInternalServerError, exit")
						results <- err.Error()
						return err
					case "StatusNoContent":
						logger.Warn("worker got StatusNoContent, sleep for 5 sec")
						time.Sleep(5 * time.Second)
						// Обнуляем "испорченный" объект Accrual и перезапускаем регистрацию accrual заново
						accrual = &external.AccrualResponse{Status: ""}
						continue
					}
					// проверяем ошибку StatusTooManyRequests -- разбиваем на 2 части, вторая часть -- таймаут, который используем в sleep
					if errSplit := strings.Split(err.Error(), " "); errSplit[0] == "StatusTooManyRequests" {
						logger.Error("worker got StatusTooManyRequests, sleep for", "time", errSplit[1])
						t, err1 := time.ParseDuration(errSplit[1])
						if err1 != nil {
							logger.Error("worker got StatusTooManyRequests, but time.ParseDuration failed with ", "error", err1)
						}
						time.Sleep(t)
					}
					logger.Error("worker: failed to register accrual", "number", orderID, "error", err)
					results <- err.Error()
					return err
				}

				// Если статус в Accrual изменился -- обновляем заказ новыми данными
				if accrual.Status != tmpStatus {
					err = store.UpdateOrderByAccrual(ctx, conf, orderID, accrual.Status, accrual.Accrual)
					if err != nil {
						logger.Error("UpdateOrderByAccrual", "Error", err)
						results <- err.Error()
						return err
					}
					message := "OrderID: " + accrual.Order + ", prev status - " + tmpStatus + ", new status - " + accrual.Status
					results <- message

					tmpStatus = accrual.Status
					logger.Info("UpdateOrderByAccrual end:", "Store", store)

				}
			}
			// пауза перед следующей итерацией
			time.Sleep(time.Duration(1) * time.Second)
		}
	}
	return nil
}

// task функция управления горутинами запроса ко внешней Accrual системе
func task(ctx context.Context, conf *initconf.Config, store handlers.Storager, wg *sync.WaitGroup, jobs <-chan string) {
	logger = conf.LogConf.Logger
	ctxWorker, cancelWorker := context.WithCancel(ctx)
	// создаем буферизованный канал для отправки результатов
	results := make(chan string, workerPoolSize)

	for w := 1; w <= workerPoolSize; w++ {
		wg.Add(1)
		logger.Debug("waitGroup", "wgAdd", w)
		go worker(ctxWorker, conf, store, w, wg, jobs, results)
	}
	logger.Debug("task: Started worker pool with", "workersNumber", workerPoolSize)

	for i := 1; i <= workerPoolSize; i++ {
		message := <-results
		logger.Debug("Message from worker", "message", message)
	}

	<-ctx.Done()
	logger.Debug("WORKER stopping")
	cancelWorker()
	wg.Wait()
	logger.Info("Main done")
	logger.Info("TASK STOPPED.")
}
