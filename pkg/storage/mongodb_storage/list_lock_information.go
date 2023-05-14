package mongodb_storage

import (
	"context"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"go.mongodb.org/mongo-driver/mongo"
)

type ListMongoLockIterator struct {
	cursor *mongo.Cursor
}

var _ iterator.Iterator[*storage.LockInformation] = &ListMongoLockIterator{}

func NewListMongoLockIterator(cursor *mongo.Cursor) *ListMongoLockIterator {
	return &ListMongoLockIterator{
		cursor,
	}
}

func (x *ListMongoLockIterator) Next() bool {
	return x.cursor.Next(context.Background())
}

func (x *ListMongoLockIterator) Value() *storage.LockInformation {
	r := &storage.LockInformation{}
	_ = x.cursor.Decode(&r)
	return r
}
