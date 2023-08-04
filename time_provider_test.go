package storage_lock

import (
	"context"
	"fmt"
	"github.com/storage-lock/go-events"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNTPTimeProvider_GetTime(t *testing.T) {

	e := events.NewEvent("xxx").AddListeners(events.NewListenerWrapper(func(ctx context.Context, e *events.Event) {
		fmt.Println(e.ToJsonString())
	}))

	for _, server := range DefaultNtpServers {
		provider := NewNTPTimeProvider(server)
		time, err := provider.GetTime(context.Background(), e)
		assert.Nil(t, err)
		assert.False(t, time.IsZero())
		if err != nil {
			fmt.Println(server)
		}
	}
}
