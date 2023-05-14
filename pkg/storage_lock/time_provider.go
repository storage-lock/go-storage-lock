package storage_lock

import (
	"context"
	"time"
)

// ------------------------------------------------- --------------------------------------------------------------------

// TimeProvider 能够提供时间的时间源
type TimeProvider interface {

	// GetTime 获取当前时间
	GetTime(ctx context.Context) (time.Time, error)
}

// ------------------------------------------------- --------------------------------------------------------------------

// TODO 2023-5-15 01:50:39 基于NTP的事件源实现
//type NTPTimeProvider struct {
//}
//
//var _ TimeProvider = &NTPTimeProvider{}
//
//// TODO
//func (x *NTPTimeProvider) GetTime(ctx context.Context) (time.Time, error) {
//	for {
//
//	}
//}

// ------------------------------------------------- --------------------------------------------------------------------
