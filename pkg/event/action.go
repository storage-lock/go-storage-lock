package event

import (
	"github.com/golang-infrastructure/go-pointer"
	"time"
)

//
const (
	ActionStorageGetName           = "Storage.GetName"
	ActionStorageInit              = "Storage.Init"
	ActionStorageUpdateWithVersion = "Storage.UpdateWithVersion"
	ActionStorageInsertWithVersion = "Storage.InsertWithVersion"
	ActionStorageDeleteWithVersion = "Storage.DeleteWithVersion"
	ActionStorageGetTime           = "Storage.GetTime"
	ActionStorageGet               = "Storage.Get"
	ActionStorageClose             = "Storage.Close"
	ActionStorageList              = "Storage.List"
)

// Action 时间可以被贴若干个标签
type Action struct {

	// 标签被创建的时间
	Time *time.Time

	// 标签的名字
	Name string

	// action可以携带一些自己单独的上下文信息之类的
	Payload string
}

func NewAction(name string) *Action {
	return &Action{
		Time: pointer.Now(),
		Name: name,
	}
}

func (x *Action) SetPayload(payload string) *Action {
	x.Payload = payload
	return x
}
