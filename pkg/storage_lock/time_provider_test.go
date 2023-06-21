package storage_lock

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNTPTimeProvider_GetTime(t *testing.T) {
	for _, server := range DefaultNtpServers {
		provider := NewNTPTimeProvider(server)
		time, err := provider.GetTime(context.Background())
		assert.Nil(t, err)
		assert.False(t, time.IsZero())
		if err != nil {
			fmt.Println(server)
		}
	}
}
