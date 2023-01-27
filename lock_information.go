package storage_lock

import (
	"encoding/json"
	"time"
)

// LockInformation 锁的相关信息，是要持久化保存到相关介质中的
type LockInformation struct {

	// 当前时谁在持有这个锁
	OwnerId string `json:"owner_id"`

	// 锁的变更版本号，乐观锁避免CAS的ABA问题
	Version Version `json:"version"`

	// 锁被锁定了几次，是为了支持可重入锁
	LockCount int `json:"lock_count"`

	// 这个锁是从啥时候开始被OwnerId所持有的，用于判断持有锁的时间
	LockBeginTime time.Time `json:"lock_begin_time"`

	// owner持有此锁的租约过期时间
	LeaseExpireTime time.Time `json:"lease_expire_time"`
}

func FromJsonString(jsonString string) (*LockInformation, error) {
	r := &LockInformation{}
	err := json.Unmarshal([]byte(jsonString), r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (x *LockInformation) ToJsonString() string {
	marshal, err := json.Marshal(x)
	if err != nil {
		return ""
	} else {
		return string(marshal)
	}
}
