package storage_lock

import (
	"testing"
	"time"
)

// TestLeaseMarginCheck 验证漏洞 I 的修复：续租刷新间隔与租约过期时间必须留足安全余量。
// 余量过小时一次续租抖动会让租约在下次刷新前过期、被他人合法抢占，破坏互斥性。
func TestLeaseMarginCheck(t *testing.T) {
	cases := []struct {
		name             string
		expireAfter      time.Duration
		refreshInterval  time.Duration
		skipMarginCheck  bool
		wantErr          bool
	}{
		{
			name: "正常-余量充足(3s/1s,余量2s)",
			expireAfter: time.Second * 3, refreshInterval: time.Second,
			wantErr: false,
		},
		{
			name: "正常-默认值(5m/30s,余量4.5m)",
			expireAfter: DefaultLeaseExpireAfter, refreshInterval: DefaultLeaseRefreshInterval,
			wantErr: false,
		},
		{
			name: "漏洞I-余量过小(3s/2.99s,余量10ms)",
			expireAfter: time.Second * 3, refreshInterval: time.Second*3 - time.Millisecond*10,
			wantErr: true, // 余量 10ms < max(1s, 1s)=1s，应拒绝
		},
		{
			name: "漏洞I-余量过小(9s/8s,余量1s<3s)",
			expireAfter: time.Second * 9, refreshInterval: time.Second * 8,
			wantErr: true, // 余量1s < max(1s, 3s)=3s，应拒绝
		},
		{
			name: "漏洞I-余量恰好等于下界(6s/4s,余量2s==2s)",
			expireAfter: time.Second * 6, refreshInterval: time.Second * 4,
			wantErr: false, // 余量2s == max(1s, 2s)=2s，临界通过
		},
		{
			name: "跳过检查-余量过小但SkipLeaseMarginCheck=true",
			expireAfter: time.Second * 3, refreshInterval: time.Second*3 - time.Millisecond*10,
			skipMarginCheck: true, wantErr: false,
		},
		{
			name: "刷新间隔>=过期时间-仍应被旧校验拒绝",
			expireAfter: time.Second * 3, refreshInterval: time.Second * 3,
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			options := NewStorageLockOptionsWithLockId("test-lease-margin")
			options.LeaseExpireAfter = c.expireAfter
			options.LeaseRefreshInterval = c.refreshInterval
			options.SkipLeaseMarginCheck = c.skipMarginCheck
			err := checkStorageLockOptions(options)
			if c.wantErr && err == nil {
				t.Fatalf("期望返回错误（漏洞I未拦截），实际 nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("期望无错误，实际: %v", err)
			}
		})
	}
}
