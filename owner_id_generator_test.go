package storage_lock

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOwnerIdGenerator_GenOwnerID(t *testing.T) {
	generator := NewOwnerIdGenerator()

	ownerIdSet := make(map[string]struct{}, 0)
	for i := 0; i < 1000; i++ {

		ownerId := generator.GenOwnerId()

		_, exists := ownerIdSet[ownerId]
		assert.False(t, exists)
		ownerIdSet[ownerId] = struct{}{}

		assert.NotEmpty(t, ownerId)
		t.Log(ownerId)
	}
}
