package storage_lock

import (
	goroutine_id "github.com/golang-infrastructure/go-goroutine-id"
	"net"
	"os"
	"strings"
)

type OwnerIdGenerator struct {
	defaultLockPrefix string
}

func NewOwnerIdGenerator() *OwnerIdGenerator {
	return &OwnerIdGenerator{
		defaultLockPrefix: generateMachineDefaultLockIdPrefix(),
	}
}

// 这个方法应该具有幂等性，同一个goroutine应该恒返回同一个ID
func (x *OwnerIdGenerator) getDefaultOwnId() string {
	return x.defaultLockPrefix + goroutine_id.GetGoroutineIdAsString()
}

// 获取当前机器的MAC地址
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
