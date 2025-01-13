package main

import (
	"context"
	"errors"
	"fmt"
	"martnew/cmd/gophermart/initconf"
	"martnew/external"
	"martnew/internal/handlers"
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
					// Проверяем ошибку StatusTooManyRequests. Вытаскиваем из ошибки таймаут, который используем в sleep
					tooManyRequestsError, ok := err.(external.TooManyRequestsError)
					if ok {
						if errors.Is(err, tooManyRequestsError) {
							logger.Error("worker got StatusTooManyRequests, sleep for", "time", tooManyRequestsError.Timeout)
							time.Sleep(time.Duration(tooManyRequestsError.Timeout) * time.Second)
						}
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
					message := fmt.Sprintf("OrderID: %s, prev status - %s, new status - %s", accrual.Order, tmpStatus, accrual.Status)
					results <- message

					tmpStatus = accrual.Status
					logger.Info("UpdateOrderByAccrual end:", "Store", store)

				}
			}
			// пауза перед следующей итерацией
			time.Sleep(time.Duration(1) * time.Second)
			logger.Debug("worker running", "workerID", workerID)
		}
	}
	return nil
}

// resultProcessing горутина обработчика результатов работы worker-ов
func resultProcessing(ctx context.Context, wg *sync.WaitGroup, results <-chan string) {
	logger.Debug("resultProcessing goroutine started")
	defer func() {
		wg.Done()
	}()
	for {
		select {
		case message := <-results:
			logger.Debug("Message from worker", "message", message)
		case <-ctx.Done():
			logger.Debug("resultProcessing goroutine finished")
			return
		default:
			time.Sleep(1 * time.Second)
		}
	}
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

	// старт горутины обработчика результатов работы worker-ов
	wg.Add(1)
	go resultProcessing(ctxWorker, wg, results)

	<-ctx.Done()
	logger.Debug("WORKER stopping")
	cancelWorker()
	wg.Wait()
	logger.Info("Main done")
	logger.Info("TASK STOPPED.")
}
