package memory_storage

import (
	"github.com/storage-lock/go-storage-lock/pkg/storage/test_helper"
	"testing"
)

func TestMemoryStorage(t *testing.T) {
	test_helper.TestStorage(t, NewMemoryStorage())
}
