package storage_lock

import (
	"fmt"
	goroutine_id "github.com/golang-infrastructure/go-goroutine-id"
	"github.com/google/uuid"
	"net"
	"os"
	"strings"
)

// OwnerIdGenerator 当没有指定锁的持有者的ID的时候，自动生成一个ID
// 1. 要满足全局唯一不重复
// 2. 可读性尽量好一些
type OwnerIdGenerator struct {
	defaultLockPrefix string
}

func NewOwnerIdGenerator() *OwnerIdGenerator {
	prefix := generateMachineDefaultLockIdPrefix()
	// 不能太长，不然存储和索引啥的费事
	prefixMaxLength := 100
	if len(prefix) > prefixMaxLength {
		prefix = prefix[0:prefixMaxLength]
	}
	return &OwnerIdGenerator{
		defaultLockPrefix: prefix,
	}
}

// 这个方法应该具有幂等性，同一个goroutine应该恒返回同一个ID
func (x *OwnerIdGenerator) getDefaultOwnId() string {
	// 这里为了可读性并没有将区分度最高的UUID放在前面，这是假设使用此分布式锁的各个竞争者的Hostname基本都不会相同
	// 因为是同一台机器上使用分布式锁不是很有意义
	return fmt.Sprintf("%s-%s-%s", x.defaultLockPrefix, goroutine_id.GetGoroutineIDAsString(), strings.ReplaceAll(uuid.New().String(), "-", ""))
}

// 获取当前机器的Hostname作为唯一ID的前缀
func generateMachineDefaultLockIdPrefix() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	macAddresses := strings.Builder{}

	// 主机名
	hostname, _ := os.Hostname()
	if hostname != "" {
		macAddresses.WriteString(hostname)
		macAddresses.WriteString("-")
	}

	// MAC地址
	for index, netInterface := range netInterfaces {
		macAddr := netInterface.HardwareAddr.String()
		if len(macAddr) == 0 {
			continue
		}
		macAddresses.WriteString(macAddr)
		if index < len(netInterfaces) {
			macAddresses.WriteString(",")
		}
	}
	macAddresses.WriteString("-")

	return macAddresses.String()
}
