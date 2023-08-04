package storage_base

import (
	"database/sql"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
)

// SqlRowsIterator 用来把sql.Row包装为一个迭代器
type SqlRowsIterator struct {
	rows *sql.Rows
}

var _ iterator.Iterator[*storage.LockInformation] = &SqlRowsIterator{}

func NewSqlRowsIterator(rows *sql.Rows) *SqlRowsIterator {
	return &SqlRowsIterator{
		rows: rows,
	}
}

func (x *SqlRowsIterator) Next() bool {
	hasNext := x.rows.Next()
	if !hasNext {
		// 当遍历完的时候把Rows给关闭掉，防止连接泄露
		_ = x.rows.Close()
	}
	return hasNext
}

func (x *SqlRowsIterator) Value() *storage.LockInformation {
	r := &storage.LockInformation{}
	_ = x.rows.Scan(&r)
	return r
}
