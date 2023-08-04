package event

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"strings"
	"time"
)

// Event 表示一个事件
type Event struct {
	ID     string
	Parent *Event

	LockId      string
	StorageName string

	StartTime, EndTime time.Time

	EventType EventType

	Actions []*Action

	WatchDogId string

	LockInformation *storage.LockInformation

	Err error

	Listeners []EventListener
}

var _ json.Marshaler = &Event{}
var _ json.Unmarshaler = &Event{}

func NewEvent(lockId string) *Event {
	return &Event{
		ID:        "event-" + strings.ReplaceAll(uuid.New().String(), "-", ""),
		LockId:    lockId,
		StartTime: time.Now(),
	}
}

func (x *Event) End() *Event {
	x.EndTime = time.Now()
	return x
}

func (x *Event) Fork() *Event {
	return &Event{
		ID:              "event-" + strings.ReplaceAll(uuid.New().String(), "-", ""),
		Parent:          x,
		LockId:          x.LockId,
		StorageName:     x.StorageName,
		StartTime:       time.Now(),
		Listeners:       x.Listeners,
		LockInformation: x.LockInformation,
	}
}

func (x *Event) SetStorageName(storageName string) *Event {
	x.StorageName = storageName
	return x
}

func (x *Event) SetLockId(lockId string) *Event {
	x.LockId = lockId
	return x
}

func (x *Event) SetType(eventType EventType) *Event {
	x.EventType = eventType
	return x
}

func (x *Event) SetErr(err error) *Event {
	x.Err = err
	return x
}

func (x *Event) AppendAction(action string) *Event {
	x.Actions = append(x.Actions, NewAction(action))
	return x
}

func (x *Event) SetEventListeners(listeners []EventListener) *Event {
	x.Listeners = listeners
	return x
}

func (x *Event) SetWatchDogId(watchDogId string) *Event {
	x.WatchDogId = watchDogId
	return x
}

func (x *Event) SetLockInformation(lockInformation *storage.LockInformation) *Event {
	x.LockInformation = lockInformation
	return x
}

// Publish 把当前的事件发布到多个Listener上
func (x *Event) Publish(ctx context.Context, listeners ...EventListener) {

	// 如果要发布的时候没有设置过结束时间，则自动设置
	if x.EndTime.IsZero() {
		x.End()
	}

	if len(x.Listeners) != 0 {
		for _, listener := range x.Listeners {
			listener.On(ctx, x)
		}
	}

	if len(listeners) != 0 {
		for _, listener := range listeners {
			listener.On(ctx, x)
		}
	}

}

func (x *Event) MarshalJSON() ([]byte, error) {
	//TODO implement me
	panic("implement me")
}

func (x *Event) UnmarshalJSON(bytes []byte) error {
	//TODO implement me
	panic("implement me")
}
