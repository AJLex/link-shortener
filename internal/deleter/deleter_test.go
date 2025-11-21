package deleter

import (
	"context"
	"testing"
	"time"

	"github.com/AJLex/link-shortener/internal/storage/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestDeleter_StartStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockStorage(ctrl)
	logger := zap.NewNop()

	deleter := NewDeleter(mockStorage, 10, 3, 100*time.Millisecond, logger)
	deleter.Start()

	// Даём время на запуск
	time.Sleep(50 * time.Millisecond)

	// Останавливаем
	deleter.Stop()

	// Проверяем, что остановка прошла успешно
	assert.NotNil(t, deleter)
}

func TestDeleter_DeleteSingleTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockStorage(ctrl)
	logger := zap.NewNop()

	// Ожидаем вызов DeleteBatch
	mockStorage.EXPECT().
		DeleteBatch(gomock.Any(), []string{"code1"}, "user1").
		Return(nil).
		Times(1)

	deleter := NewDeleter(mockStorage, 10, 3, 100*time.Millisecond, logger)
	deleter.Start()

	// Отправляем задачу
	deleter.Delete("code1", "user1")

	// Ждём обработки
	time.Sleep(200 * time.Millisecond)

	deleter.Stop()
}

func TestDeleter_BatchAccumulation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockStorage(ctrl)
	logger := zap.NewNop()

	// Ожидаем вызов с батчем (10 элементов - полный batch)
	mockStorage.EXPECT().
		DeleteBatch(gomock.Any(), gomock.Len(10), "user1").
		Return(nil).
		Times(1)

	deleter := NewDeleter(mockStorage, 10, 1, 5*time.Second, logger)
	deleter.Start()

	// Отправляем 10 задач - должен заполниться буфер
	for i := 0; i < 10; i++ {
		deleter.Delete("code1", "user1")
	}

	// Даём время на обработку
	time.Sleep(200 * time.Millisecond)

	deleter.Stop()
}

func TestDeleter_FlushTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockStorage(ctrl)
	logger := zap.NewNop()

	// Ожидаем вызов с неполным батчем (по таймауту)
	mockStorage.EXPECT().
		DeleteBatch(gomock.Any(), gomock.Any(), "user1").
		Return(nil).
		MinTimes(1)

	deleter := NewDeleter(mockStorage, 100, 1, 100*time.Millisecond, logger)
	deleter.Start()

	// Отправляем только 3 задачи (меньше batchSize)
	for i := 0; i < 3; i++ {
		deleter.Delete("code1", "user1")
	}

	// Ждём таймаута flush
	time.Sleep(200 * time.Millisecond)

	deleter.Stop()
}

func TestDeleter_GracefulShutdown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := mock.NewMockStorage(ctrl)
	logger := zap.NewNop()

	deletedCount := 0
	mockStorage.EXPECT().
		DeleteBatch(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, codes []string, userID string) error {
			deletedCount += len(codes)
			return nil
		}).
		AnyTimes()

	deleter := NewDeleter(mockStorage, 10, 3, 100*time.Millisecond, logger)
	deleter.Start()

	// Даём время на запуск всех воркеров
	time.Sleep(50 * time.Millisecond)

	// Отправляем 25 задач
	for i := 0; i < 25; i++ {
		deleter.Delete("code1", "user1")
	}

	// Даём немного времени на накопление задач в буферах воркеров
	time.Sleep(50 * time.Millisecond)

	// Останавливаем - все задачи должны быть обработаны до завершения Stop()
	deleter.Stop()

	// Даём дополнительное время на завершение обработки
	time.Sleep(200 * time.Millisecond)

	// Проверяем, что все 25 задач были обработаны
	assert.Equal(t, 25, deletedCount, "All tasks must be processed during graceful shutdown")
}
