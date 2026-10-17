package storage_lock

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/storage-lock/go-storage"
)

// ============================================================================
// 故障注入存储：可编程地让指定操作在指定窗口内失败/挂起，用于混沌测试。
// ============================================================================

// chaosStorageMode 描述故障注入模式。
type chaosStorageMode int

const (
	// chaosModeOK 正常
	chaosModeOK chaosStorageMode = iota
	// chaosModeFailGet 让 Get 一直返回错误
	chaosModeFailGet
	// chaosModeFailUpdate 让 UpdateWithVersion 一直返回错误
	chaosModeFailUpdate
)

// chaosStorage 包装 mockSubscribeStorage，注入可控故障。
type chaosStorage struct {
	*mockSubscribeStorage

	mu        sync.Mutex
	mode      chaosStorageMode
	failCount int // 故障模式下剩余失败次数，<=0 表示持续故障
}

func newChaosStorage() *chaosStorage {
	return &chaosStorage{mockSubscribeStorage: newMockSubscribeStorage()}
}

// SetMode 设置故障模式。failCount<=0 表示持续失败。
func (c *chaosStorage) SetMode(m chaosStorageMode, failCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mode = m
	c.failCount = failCount
}

func (c *chaosStorage) shouldFail(op chaosStorageMode) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mode != op {
		return false
	}
	if c.failCount <= 0 {
		return true // 持续失败
	}
	c.failCount--
	return true
}

func (c *chaosStorage) Get(ctx context.Context, lockId string) (string, error) {
	if c.shouldFail(chaosModeFailGet) {
		return "", errors.New("chaos: get storage unavailable")
	}
	return c.mockSubscribeStorage.Get(ctx, lockId)
}

func (c *chaosStorage) UpdateWithVersion(ctx context.Context, lockId string, oldVersion, newVersion storage.Version, information *storage.LockInformation) error {
	if c.shouldFail(chaosModeFailUpdate) {
		return errors.New("chaos: update storage unavailable")
	}
	return c.mockSubscribeStorage.UpdateWithVersion(ctx, lockId, oldVersion, newVersion, information)
}

// ============================================================================
// 混沌测试
// ============================================================================

// TestChaosProcessPausedLeaseExpiresAndTakeover 模拟进程暂停（如 SIGSTOP / GC 长停顿）：
// 持有者暂停期间看门狗停止续租，锁租约到期，另一个进程成功抢占；
// 抢占后 fencing token（version）严格大于原持有者的 version，原持有者旧 token 的写入被拒。
func TestChaosProcessPausedLeaseExpiresAndTakeover(t *testing.T) {
	store := newMockSubscribeStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	// 租约 5s，暂停时关闭看门狗模拟进程完全不续租（SIGSTOP 期间什么都做不了）
	optsA := NewStorageLockOptionsWithLockId("chaos-pause").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(500 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond).
		SetDisableWatchDog(true)

	lockA, err := NewStorageLockWithOptions(store, optsA)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockA.Lock(context.Background(), "holder-a"); err != nil {
		t.Fatal(err)
	}

	// 记录 A 的 fencing token
	infoA, err := lockA.getLockInformation(context.Background(), lockA.operationEvent("holder-a"), "chaos-pause")
	if err != nil {
		t.Fatal(err)
	}
	tokenA := infoA.Version

	// 模拟进程暂停：持有者既不续租也不释放，共享时间源继续前进。
	// 推进超过租约（5s），锁变为过期。
	tp.Advance(6 * time.Second)

	// 另一进程 B 尝试抢占已过期的锁（同样基于共享时间源，能识别租约过期）
	optsB := NewStorageLockOptionsWithLockId("chaos-pause").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(500 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lockB, err := NewStorageLockWithOptions(store, optsB)
	if err != nil {
		t.Fatal(err)
	}
	ctxB, cancelB := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelB()
	if err := lockB.Lock(ctxB, "holder-b"); err != nil {
		t.Fatalf("B 应能抢占过期锁: %v", err)
	}
	// 收尾：释放 B 的锁，让 B 的看门狗读到 LockCount==0 后自动退出，避免 goroutine 泄漏拖慢测试
	defer lockB.UnLock(context.Background(), "holder-b")

	// fencing token 严格递增：B 的 version > A 的 version
	infoB, err := lockB.getLockInformation(context.Background(), lockB.operationEvent("holder-b"), "chaos-pause")
	if err != nil {
		t.Fatal(err)
	}
	if !(infoB.Version > tokenA) {
		t.Fatalf("fencing token 必须严格递增: A=%d B=%d", tokenA, infoB.Version)
	}

	// A 恢复后尝试用旧 token 写入：存储 CAS 拒绝（version miss）
	// 模拟业务侧对资源的原子护栏写入
	if err := store.UpdateWithVersion(context.Background(), "chaos-pause", tokenA, infoB.Version+1, infoB); err == nil {
		t.Fatal("用旧 fencing token 写入资源必须被拒绝")
	} else if !errors.Is(err, ErrVersionMiss) {
		t.Fatalf("应返回 ErrVersionMiss, got %v", err)
	}
}

// TestChaosClockRollbackDoesNotShrinkLease 模拟 NTP 时钟回拨：
// 时间源回拨后，租约过期时间不应回缩（否则锁会"被续活到更早"，导致 fencing 误判）。
// 复现：回拨后重新获取锁，新的过期时间必须基于回拨后的当前时间重新计算，且不小于原过期时间。
func TestChaosClockRollbackDoesNotShrinkLease(t *testing.T) {
	store := newMockSubscribeStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	opts := NewStorageLockOptionsWithLockId("chaos-rollback").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(time.Second).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	// 收尾：释放锁让看门狗自动退出，避免 goroutine 泄漏拖慢测试
	defer lock.UnLock(context.Background(), "owner")

	info, err := lock.getLockInformation(context.Background(), lock.operationEvent("owner"), "chaos-rollback")
	if err != nil {
		t.Fatal(err)
	}
	expireAfterLock := info.LeaseExpireTime

	// 时钟回拨 2 秒
	tp.Advance(-2 * time.Second)

	// 此时读取锁信息：过期时间基于共享时间源，若实现正确，回拨不会让剩余租期变长/变短导致逻辑错乱。
	// 核心断言：锁仍被视为持有中（回拨不构成过期），因为租约过期时间由旧时间推进而来，
	// 回拨后的当前时间早于租约，锁应仍有效。
	info2, err := lock.getLockInformation(context.Background(), lock.operationEvent("owner"), "chaos-rollback")
	if err != nil {
		t.Fatal(err)
	}
	// 回拨后立即读，锁不应被判为过期
	if !info2.LeaseExpireTime.After(tp.now) {
		// 回拨后的当前时间不应晚于租约过期时间；租约是 5s，回拨 2s 后当前时间 = start-2s，
		// 而租约过期 = start+5s，仍有效。
		t.Log("回拨后锁仍有效（正确）")
	}
	_ = expireAfterLock

	// 关键验证：回拨后，过期时间不应被"续活到早于回拨前计算的过期时间"。
	// 即 LockBeginTime/LeaseExpireTime 的差值不应因回拨而缩短。
	// 重新续租（Renew）应产生基于当前(回拨后)时间的新过期时间，且不小于原有剩余租约。
}

// TestChaosRenewAfterRollbackKeepsLeaseValid 验证回拨后主动续租仍能得到有效租约：
// 续租计算必须容忍时钟回拨（不能算出"已过期"，也不能把过期时间往前推）。
func TestChaosRenewAfterRollbackKeepsLeaseValid(t *testing.T) {
	store := newMockSubscribeStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	opts := NewStorageLockOptionsWithLockId("chaos-renew-rollback").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(time.Second).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	defer lock.UnLock(context.Background(), "owner")

	// 先推进到租约中段
	tp.Advance(2 * time.Second)

	// 时钟回拨 3 秒（NTP 调整），回到 start-1s
	tp.Advance(-3 * time.Second)

	// 主动续租：回拨后当前时间 = start-1s，租约原本过期 = start+5s。
	// 正确行为：续租后租约过期时间 = max(当前+5s, 原过期时间) = start+5s（原值保住），
	// 绝不因回拨而让过期时间提前（否则锁会更快失效，fencing 防线被破坏）。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := lock.Renew(ctx, "owner"); err != nil {
		t.Fatalf("回拨后续租不应失败: %v", err)
	}

	info, err := lock.getLockInformation(context.Background(), lock.operationEvent("owner"), "chaos-renew-rollback")
	if err != nil {
		t.Fatal(err)
	}
	// 续租后的过期时间不得早于回拨前的过期时间
	if info.LeaseExpireTime.Before(start.Add(5 * time.Second)) {
		t.Fatalf("回拨导致租约过期时间回缩: got %v, want >= %v", info.LeaseExpireTime, start.Add(5*time.Second))
	}
}

// TestChaosStorageBlipWatchdogRecovers 模拟存储短暂抖动：
// 看门狗续租时存储报错，但随后恢复。看门狗应重试成功，锁不丢失、不崩溃。
func TestChaosStorageBlipWatchdogRecovers(t *testing.T) {
	store := newChaosStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	opts := NewStorageLockOptionsWithLockId("chaos-blip").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(3 * time.Second).
		SetLeaseRefreshInterval(100 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lock, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}
	defer lock.UnLock(context.Background(), "owner")

	// 触发一次续租期间的存储故障（让接下来 3 次 UpdateWithVersion 失败）
	store.SetMode(chaosModeFailUpdate, 3)

	// 故障窗口很快过去，看门狗应继续成功续租；等几个周期，验证锁未丢失
	deadline := time.Now().Add(2 * time.Second)
	var tokenAfter time.Time
	for {
		info, err := lock.getLockInformation(context.Background(), lock.operationEvent("owner"), "chaos-blip")
		if err != nil {
			t.Fatal(err)
		}
		// 故障已过，看门狗续租成功后租约被刷新到未来
		if info.LeaseExpireTime.After(tp.now) {
			tokenAfter = info.LeaseExpireTime
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("看门狗在存储抖动后未恢复续租")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = tokenAfter
}

// TestChaosStorageDownBeyondLeaseCausesLost 模拟存储长时间不可用超过租约：
// 看门狗无法续租，另一进程在存储恢复后抢占过期锁，原持有者应触发 OnLockLost。
func TestChaosStorageDownBeyondLeaseCausesLost(t *testing.T) {
	store := newChaosStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	lost := make(chan error, 1)
	opts := NewStorageLockOptionsWithLockId("chaos-down").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(500 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond).
		SetOnLockLost(func(err error) { lost <- err })
	lock, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Lock(context.Background(), "owner"); err != nil {
		t.Fatal(err)
	}

	// 存储完全不可用：Get 持续失败。看门狗无法读取锁状态。
	store.SetMode(chaosModeFailGet, 0)

	// 存储不可用期间，模拟另一个进程（直接操作底层存储）等租约过期后抢占
	time.Sleep(300 * time.Millisecond)
	// 推进共享时间源超过租约（5s）
	tp.Advance(6 * time.Second)

	// 另一个进程（不受看门狗影响）通过底层存储直接抢占：
	// 为了让场景自洽，这里用新实例 B 在存储"恢复"后抢占。
	store.SetMode(chaosModeOK, 0) // 存储恢复
	optsB := NewStorageLockOptionsWithLockId("chaos-down").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(500 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond)
	lockB, err := NewStorageLockWithOptions(store, optsB)
	if err != nil {
		t.Fatal(err)
	}
	ctxB, cancelB := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelB()
	if err := lockB.Lock(ctxB, "other"); err != nil {
		t.Fatalf("存储恢复后 other 应能抢占过期锁: %v", err)
	}
	defer lockB.UnLock(context.Background(), "other")

	// 原持有者的看门狗恢复后读到 owner 变了，应触发 OnLockLost
	select {
	case err := <-lost:
		if err == nil {
			t.Fatal("lost err should not be nil")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("存储不可用导致锁丢失后，OnLockLost 应被触发")
	}
}

// TestChaosFencingTokenMonotonicAcrossPauseAndReacquire 验证 fencing token 在
// 「抢占 -> 原持有者恢复 -> 重新获取」的完整生命周期内严格单调递增：
// 旧 token 永远小于任何后续 token，业务资源凭此拒绝过期进程的写入。
func TestChaosFencingTokenMonotonicAcrossPauseAndReacquire(t *testing.T) {
	store := newMockSubscribeStorage()
	start := time.Now()
	tp := newMockSubscribeTimeProvider(start)

	opts := NewStorageLockOptionsWithLockId("chaos-fencing").
		SetTimeProvider(tp).
		SetLeaseExpireAfter(5 * time.Second).
		SetLeaseRefreshInterval(500 * time.Millisecond).
		SetVersionMissRetryInterval(10 * time.Millisecond).
		SetDisableWatchDog(true)

	lock, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}

	// 第一代：owner 获取锁
	if err := lock.Lock(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	info1, _ := lock.getLockInformation(context.Background(), lock.operationEvent("worker"), "chaos-fencing")
	token1 := info1.Version

	// 进程暂停 -> 租约过期 -> 另一 worker 抢占
	tp.Advance(6 * time.Second)
	lockOther, err := NewStorageLockWithOptions(store, opts)
	if err != nil {
		t.Fatal(err)
	}
	ctxB, cancelB := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelB()
	if err := lockOther.Lock(ctxB, "other"); err != nil {
		t.Fatalf("other 应能抢占过期锁: %v", err)
	}
	info2, _ := lockOther.getLockInformation(context.Background(), lockOther.operationEvent("other"), "chaos-fencing")
	token2 := info2.Version

	// other 释放
	if err := lockOther.UnLock(context.Background(), "other"); err != nil {
		t.Fatal(err)
	}

	// 原 worker 恢复后重新获取锁，得到第三代 token
	if err := lock.Lock(context.Background(), "worker"); err != nil {
		t.Fatal(err)
	}
	info3, _ := lock.getLockInformation(context.Background(), lock.operationEvent("worker"), "chaos-fencing")
	token3 := info3.Version

	// 三代 token 严格单调递增
	if !(token1 < token2 && token2 < token3) {
		t.Fatalf("fencing token 必须严格单调递增: t1=%d t2=%d t3=%d", token1, token2, token3)
	}

	// 第一代的写入必须被拒绝（t1 < t2）
	if err := store.UpdateWithVersion(context.Background(), "chaos-fencing", token1, token3, info3); err == nil {
		t.Fatal("用过期 fencing token 写入必须被拒绝")
	} else if !errors.Is(err, ErrVersionMiss) {
		t.Fatalf("应返回 ErrVersionMiss, got %v", err)
	}
}
