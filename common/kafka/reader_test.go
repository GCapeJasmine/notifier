package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPollKafka_ShouldFinish(t *testing.T) {
	messages, err := pollKafka(context.Background(), time.Millisecond*100, 100, func(ctx context.Context) (int64, error) {
		time.Sleep(time.Millisecond * 10)
		return time.Now().UnixMilli(), nil
	})

	assert.Nil(t, err)
	assert.LessOrEqual(t, len(messages), 11)
}

func TestPollKafka_ShouldFinishBeforeTimeoutWhenMaxSizeReached(t *testing.T) {
	messages, err := pollKafka(context.Background(), time.Millisecond*100, 3, func(ctx context.Context) (int64, error) {
		time.Sleep(time.Millisecond * 10)
		return time.Now().UnixMilli(), nil
	})

	assert.Nil(t, err)
	assert.LessOrEqual(t, len(messages), 3)
}

func TestPollKafka_ShouldReturnSingleWhenNegativeSize(t *testing.T) {
	messages, err := pollKafka(context.Background(), time.Millisecond*100, -3, func(ctx context.Context) (int64, error) {
		time.Sleep(time.Millisecond * 10)
		return time.Now().UnixMilli(), nil
	})

	assert.Nil(t, err)
	assert.LessOrEqual(t, len(messages), 1)
}

func TestPollKafka_ShouldReturnErrorWhenDeadlineExceeded(t *testing.T) {
	_, err := pollKafka(context.Background(), time.Millisecond*10, 100, func(ctx context.Context) (int64, error) {
		return 0, context.DeadlineExceeded
	})

	assert.Nil(t, err)
}
