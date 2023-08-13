package storage_lock

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	goroutine_id "github.com/golang-infrastructure/go-goroutine-id"
	"github.com/storage-lock/go-utils"
	"net"
	"os"
	"strings"
)

// OwnerIdGenerator 可以使用这个工具类来生成全局唯一的OwnerID
// 1. 要满足全局唯一不重复
// 2. 可读性尽量好一些
type OwnerIdGenerator struct {
	defaultLockPrefix string
}

// NewOwnerIdGenerator 创建一个OwnerID生成器
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

// GenOwnerId 这个方法应该具有幂等性，同一个goroutine应该恒返回同一个ID
func (x *OwnerIdGenerator) GenOwnerId() string {
	// 这里为了可读性并没有将区分度最高的UUID放在前面，这是假设使用此分布式锁的各个竞争者的Hostname基本都不会相同
	// 因为是同一台机器上使用分布式锁不是很有意义
	return fmt.Sprintf("%s-%s-%s", x.defaultLockPrefix, goroutine_id.GetGoroutineIDAsString(), utils.RandomID())
}

// 获取当前机器的Hostname作为唯一ID的前缀
func generateMachineDefaultLockIdPrefix() string {
	netInterfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	parts := make([]string, 0)
	parts = append(parts, "storage-lock-owner-id")

	// 主机名
	hostname, _ := os.Hostname()
	if hostname != "" {
		parts = append(parts, hostname)
	}

	// MAC地址
	macAddresses := strings.Builder{}
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
	h := md5.New()
	h.Write([]byte(macAddresses.String()))
	macMd5 := hex.EncodeToString(h.Sum(nil))
	parts = append(parts, macMd5)

	return strings.Join(parts, "-")
}
