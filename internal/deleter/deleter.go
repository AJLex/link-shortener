package deleter

import (
	"context"
	"sync"
	"time"

	models "github.com/AJLex/link-shortener/internal/model"
	"github.com/AJLex/link-shortener/internal/storage"
	"go.uber.org/zap"
)

// Deleter асинхронный обработчик удаления URL с использованием Fan-In паттерна
type Deleter struct {
	storage       storage.Storage
	inputChan     chan models.DeleteTask
	fanInChan     chan []models.DeleteTask
	batchSize     int
	flushTimeout  time.Duration
	workers       int
	wg            sync.WaitGroup
	processorDone chan struct{} // сигнал о завершении batch processor
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
}

// NewDeleter создает новый Deleter
func NewDeleter(
	storage storage.Storage,
	batchSize int,
	workers int,
	flushTimeout time.Duration,
	logger *zap.Logger,
) *Deleter {
	ctx, cancel := context.WithCancel(context.Background())

	return &Deleter{
		storage:       storage,
		inputChan:     make(chan models.DeleteTask, 100),
		fanInChan:     make(chan []models.DeleteTask, workers),
		batchSize:     batchSize,
		flushTimeout:  flushTimeout,
		workers:       workers,
		processorDone: make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
		logger:        logger,
	}
}

// Start запускает все воркеры и batch processor
func (d *Deleter) Start() {
	d.logger.Info("Starting deleter",
		zap.Int("workers", d.workers),
		zap.Int("batchSize", d.batchSize),
		zap.Duration("flushTimeout", d.flushTimeout))

	// Запускаем N воркеров, которые накапливают задачи
	d.wg.Add(d.workers)
	for i := 0; i < d.workers; i++ {
		go d.fanInWorker(i)
	}

	// Запускаем горутину для закрытия fanInChan после завершения всех воркеров
	// Это ключевой момент Fan-In паттерна!
	go func() {
		d.wg.Wait()
		close(d.fanInChan)
		d.logger.Info("All fan-in workers stopped, fanInChan closed")
	}()

	// Запускаем batch processor (читает из fanInChan)
	go d.batchProcessor()
}

// Delete добавляет задачу на удаление (вызывается из HTTP handler)
func (d *Deleter) Delete(shortCode, userID string) {
	select {
	case d.inputChan <- models.DeleteTask{ShortCode: shortCode, UserID: userID}:
		// Успешно отправили
	case <-d.ctx.Done():
		// Deleter уже останавливается, игнорируем
		d.logger.Warn("Deleter is stopping, task ignored",
			zap.String("shortCode", shortCode))
	default:
		// Канал полон - не блокируемся, чтобы не зависнуть HTTP handler
		// Это предотвращает deadlock при graceful shutdown
		d.logger.Warn("Input channel full, task dropped",
			zap.String("shortCode", shortCode),
			zap.String("userID", userID))
	}
}

// fanInWorker - воркер, который накапливает задачи в буфер
func (d *Deleter) fanInWorker(id int) {
	defer d.wg.Done()

	buffer := make([]models.DeleteTask, 0, d.batchSize)
	timer := time.NewTimer(d.flushTimeout)
	defer timer.Stop()

	d.logger.Info("Fan-in worker started", zap.Int("workerID", id))

	flush := func() {
		if len(buffer) > 0 {
			// Отправляем накопленный батч в fanInChan
			d.fanInChan <- buffer
			d.logger.Debug("Worker flushed batch",
				zap.Int("workerID", id),
				zap.Int("batchSize", len(buffer)))

			// Создаём новый буфер
			buffer = make([]models.DeleteTask, 0, d.batchSize)
			timer.Reset(d.flushTimeout)
		}
	}

	for {
		select {
		case task, ok := <-d.inputChan:
			if !ok {
				// inputChan закрыт - отправляем последний батч и выходим
				flush()
				d.logger.Info("Worker stopped gracefully", zap.Int("workerID", id))
				return
			}

			buffer = append(buffer, task)

			// Если буфер заполнен - отправляем батч
			if len(buffer) >= d.batchSize {
				flush()
			}

		case <-timer.C:
			// Таймаут истёк - отправляем то, что накопили
			flush()

		case <-d.ctx.Done():
			// Экстренная остановка
			flush()
			d.logger.Info("Worker stopped by context", zap.Int("workerID", id))
			return
		}
	}
}

// batchProcessor обрабатывает батчи из fanInChan
func (d *Deleter) batchProcessor() {
	defer close(d.processorDone) // Сигнализируем о завершении

	d.logger.Info("Batch processor started")

	// Читаем из fanInChan до его закрытия
	for batch := range d.fanInChan {
		if len(batch) == 0 {
			continue
		}

		// Группируем по userID для batch update
		userBatches := make(map[string][]string)
		for _, task := range batch {
			userBatches[task.UserID] = append(userBatches[task.UserID], task.ShortCode)
		}

		// Для каждого пользователя делаем batch update
		for userID, shortCodes := range userBatches {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			err := d.storage.DeleteBatch(ctx, shortCodes, userID)
			if err != nil {
				d.logger.Error("Failed to delete batch",
					zap.String("userID", userID),
					zap.Int("count", len(shortCodes)),
					zap.Error(err))
			} else {
				d.logger.Info("Batch deleted successfully",
					zap.String("userID", userID),
					zap.Int("count", len(shortCodes)))
			}

			cancel()
		}
	}

	d.logger.Info("Batch processor stopped (fanInChan closed)")
}

// Stop выполняет graceful shutdown
func (d *Deleter) Stop() {
	d.logger.Info("Stopping deleter...")

	// 1. Закрываем inputChan - больше не принимаем новые задачи
	close(d.inputChan)

	// 2. Отменяем context (на случай если воркеры зависли)
	d.cancel()

	// 3. Ждём завершения всех fan-in воркеров
	// После этого fanInChan будет автоматически закрыт
	d.wg.Wait()
	d.logger.Info("All workers stopped")

	// 4. Ждём завершения batch processor
	<-d.processorDone
	d.logger.Info("Batch processor stopped")

	d.logger.Info("Deleter stopped completely")
}
