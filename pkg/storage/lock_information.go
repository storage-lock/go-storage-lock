package storage

import (
	"encoding/json"
	"time"
)

// LockInformation 锁的相关信息，是要持久化保存到相关介质中的
type LockInformation struct {

	// 锁的ID
	LockId string `json:"lock_id"`

	// 当前时谁在持有这个锁，是一个全局唯一的ID
	OwnerId string `json:"owner_id"`

	// 锁的变更版本号，乐观锁避免CAS的ABA问题
	Version Version `json:"version"`

	// 锁被锁定了几次，是为了支持可重入锁，在释放锁的时候会根据加锁的次数来决定是否真正的释放锁还是就减少一次锁定次数
	LockCount int `json:"lock_count"`

	// 这个锁是从啥时候开始被OwnerId所持有的，用于判断持有锁的时间
	LockBeginTime time.Time `json:"lock_begin_time"`

	// 锁的owner持有此锁的租约过期时间，
	LeaseExpireTime time.Time `json:"lease_expire_time"`
}

// FromJsonString 从JSON字符串反序列化锁的信息
func FromJsonString(jsonString string) (*LockInformation, error) {
	r := &LockInformation{}
	err := json.Unmarshal([]byte(jsonString), r)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ToJsonString 把当前锁的信息序列化为JSON字符串以便存储到数据库
func (x *LockInformation) ToJsonString() string {
	marshal, err := json.Marshal(x)
	if err != nil {
		return ""
	} else {
		return string(marshal)
	}
}
