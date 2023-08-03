package storage_base

import (
	"database/sql"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

type RowsIterator struct {
	rows *sql.Rows
}

var _ iterator.Iterator[*storage.LockInformation] = &RowsIterator{}

func NewRowsIterator(rows *sql.Rows) *RowsIterator {
	return &RowsIterator{
		rows: rows,
	}
}

func (x *RowsIterator) Next() bool {
	hasNext := x.rows.Next()
	if !hasNext {
		// 当遍历完的时候把Rows给关闭掉，防止链接泄露
		_ = x.rows.Close()
	}
	return hasNext
}

func (x *RowsIterator) Value() *storage.LockInformation {
	r := &storage.LockInformation{}
	_ = x.rows.Scan(&r)
	return r
}
