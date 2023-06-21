package storage_lock

import (
	"context"
	"errors"
	"github.com/beevik/ntp"
	"time"
)

// ------------------------------------------------- --------------------------------------------------------------------

// TimeProvider 能够提供时间的时间源，可以从数据库读取，也可以从NTP服务器上读取
type TimeProvider interface {

	// GetTime 获取当前时间
	GetTime(ctx context.Context) (time.Time, error)
}

// ------------------------------------------------- --------------------------------------------------------------------

// DefaultNtpServers 默认的NTP服务器
var DefaultNtpServers = []string{
	"time.windows.com",
	"time.nist.gov",
	"ntp.ntsc.ac.cn",
	"ntp.aliyun.com",
	"time1.cloud.tencent.com",
	"time2.cloud.tencent.com",
	"time3.cloud.tencent.com",
	"time4.cloud.tencent.com",
	"time5.cloud.tencent.com",
}

// NTPTimeProvider 基于NTP的时间源实现
type NTPTimeProvider struct {
	ntpServers []string
}

var _ TimeProvider = &NTPTimeProvider{}

// NewNTPTimeProvider 如果是在云环境内网的话，手动指定一个内网的ntp服务器速度会更快，云服务商一般都会提供内网的ntp服务器
func NewNTPTimeProvider(ntpServers ...string) *NTPTimeProvider {
	if len(ntpServers) == 0 {
		ntpServers = DefaultNtpServers
	}
	return &NTPTimeProvider{
		ntpServers: ntpServers,
	}
}

// GetTime 从NTP获取时间，当不方便从Storage获取时间的时候可以使用NTP作为时间源
func (x *NTPTimeProvider) GetTime(ctx context.Context) (time.Time, error) {
	for _, server := range x.ntpServers {
		now, err := ntp.Time(server)
		if err != nil || now.IsZero() {
			continue
		}
		return now, nil
	}
	return time.Time{}, errors.New("get ntp time failed")
}

// ------------------------------------------------- --------------------------------------------------------------------
