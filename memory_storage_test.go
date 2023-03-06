package storage_lock

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMemoryStorage(t *testing.T) {

	lockId := "test-lock"

	lock := NewStorageLockUseMemory(lockId)

	ownerId := "test-owner"
	err := lock.Lock(context.Background(), ownerId)
	assert.Nil(t, err)

	err = lock.UnLock(context.Background(), ownerId)
	assert.Nil(t, err)

}
