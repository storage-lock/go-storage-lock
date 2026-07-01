package storage_lock

import (
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
)

// TestComputeRefreshSleepDurationLowerBound 验证漏洞 C 的修复：
// 当一次刷新耗时超过 LeaseRefreshInterval 时，computeRefreshSleepDuration
// 不能返回负值或过小值，否则 time.After(负) 立即触发，看门狗无间隔疯狂重试打爆存储。
func TestComputeRefreshSleepDurationLowerBound(t *testing.T) {
	cases := []struct {
		name           string
		refresh        time.Duration // 模拟一次刷新的耗时
		interval       time.Duration // LeaseRefreshInterval
		wantAtLeast    time.Duration // 期望 sleep 不低于此值
	}{
		{"刷新快于间隔-正常", 10 * time.Millisecond, time.Second, 500 * time.Millisecond}, // 半间隔
		{"刷新等于间隔-临界", time.Second, time.Second, 500 * time.Millisecond},
		{"刷新超过间隔-漏洞C场景", 2 * time.Second, time.Second, 500 * time.Millisecond}, // 不得变负/零
		{"刷新远超间隔", 10 * time.Second, time.Second, 500 * time.Millisecond},
		{"间隔配置很小-兜底", 5 * time.Millisecond, 5 * time.Millisecond, 100 * time.Millisecond}, // 最小兜底
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wd := &WatchDogCommonsImpl{
				storageLock: &StorageLock{
					options: &StorageLockOptions{
						LeaseRefreshInterval: c.interval,
					},
				},
			}
			// refreshBeginTime 设为现在往前推 c.refresh，模拟刷新已耗时 c.refresh
			refreshBeginTime := time.Now().Add(-c.refresh)
			got := wd.computeRefreshSleepDuration(refreshBeginTime)
			if got < c.wantAtLeast {
				t.Fatalf("sleep %v 低于下界 %v（漏洞C未修复：刷新耗时 %v 超过间隔 %v 时会疯狂重试）",
					got, c.wantAtLeast, c.refresh, c.interval)
			}
		})
	}
}

// 防止 storage 包未使用告警
var _ = storage.Version(0)
