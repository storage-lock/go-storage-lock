# Bug Fix: 三处关键缺陷修复

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development`
> Steps use checkbox (`- [ ]`) syntax.

**Symptom:**
1. 加锁时若存在残留看门狗，触发 nil pointer panic
2. 看门狗续租永远使用空 lockId 查询，导致锁租约无法续期，锁过期后互斥性被破坏
3. 看门狗退出事件在启动时立刻触发且计数器永远为 0

**Root Cause:**
1. `storage_lock_lock.go:290-291` — `x.storageLockWatchDog = nil` 在 `Stop()` 之前执行，导致 nil 接口方法调用 panic
2. `watch_dog_commons_impl.go:55-61` — `lockId` 只赋值给了局部变量，未赋值给结构体字段 `lockId`，导致续租时 `x.lockId == ""`
3. `watch_dog_commons_impl.go:89-94` — 退出事件用独立 goroutine 而非 defer，导致立即执行且闭包捕获的是初始值 0

**Impact:** 所有使用该分布式锁的场景。缺陷 1 和 2 为严重级别，直接影响锁的正确性。

**Scope:** Tiny
**Risk:** Medium（修改核心锁逻辑，但每个修复都极小且明确）
**Risks:**
- 修复涉及核心锁逻辑，需确保不引入新问题 → 缓解：逐个修复并运行全量测试

**Autonomy Level:** Full

---

### Task 1: 修复 nil pointer dereference — 加锁路径中看门狗 Stop 顺序错误

**Depends on:** None
**Files:**
- Modify: `storage_lock_lock.go:288-298`

- [ ] **Step 1: 修改 lockNotExists 中看门狗停止逻辑 — 将 Stop() 调整到置 nil 之前**

文件: `storage_lock_lock.go:288-298`（`lockNotExists` 函数中看门狗停止区块）

```go
// 插入成功，看下如果之前有续租协程的话就停掉，这一步是为了防止之前有资源未清理干净
if x.storageLockWatchDog != nil {
	stopLastWatchDogEvent := e.Fork().AddActionByName(ActionWatchDogStop).SetWatchDogId(x.storageLockWatchDog.GetID())
	err := x.storageLockWatchDog.Stop(ctx)
	// 把指针清空，防止后续被重复设置为nil
	x.storageLockWatchDog = nil
	if err != nil {
		stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopError).SetErr(err))
	} else {
		stopLastWatchDogEvent.AddAction(events.NewAction(ActionWatchDogStopSuccess))
	}
	stopLastWatchDogEvent.Publish(ctx)
}
```

- [ ] **Step 2: 验证编译通过**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "cannot"

- [ ] **Step 3: 质量门禁检查**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go vet ./... && go test ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "panic"

- [ ] **Step 4: 提交**
Run: `git add storage_lock_lock.go && git commit -m "fix(lock): stop watchdog before setting pointer to nil in lockNotExists

Previously x.storageLockWatchDog was set to nil before calling Stop(),
causing a nil pointer dereference panic. Reordered to match the correct
pattern used in storage_lock_unlock.go."`

---

### Task 2: 修复 WatchDogCommonsImpl.lockId 未赋值 — 续租用空 lockId 导致失效

**Depends on:** None
**Files:**
- Modify: `watch_dog_commons_impl.go:55-61`

- [ ] **Step 1: 修改 NewWatchDogCommonsImpl 结构体初始化 — 补充 lockId 字段赋值**

文件: `watch_dog_commons_impl.go:55-61`（`NewWatchDogCommonsImpl` 返回结构体初始化区块）

```go
	return &WatchDogCommonsImpl{
		id:          id,
		lockId:      lockId,
		isRunning:   atomic.Bool{},
		storageLock: lock,
		ownerId:     ownerId,
		e:           e,
	}
```

- [ ] **Step 2: 验证编译通过**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "cannot"

- [ ] **Step 3: 质量门禁检查**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go vet ./... && go test ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "panic"

- [ ] **Step 4: 提交**
Run: `git add watch_dog_commons_impl.go && git commit -m "fix(watchdog): assign lockId to WatchDogCommonsImpl struct field

The lockId was assigned to a local variable but never set on the struct,
causing x.lockId to always be an empty string. This broke lease refresh
as getLockInformation and UpdateWithVersion were called with lockId=."`

---

### Task 3: 修复看门狗退出事件 — 改为 defer 并捕获最终计数

**Depends on:** None
**Files:**
- Modify: `watch_dog_commons_impl.go:81-105`（Start 方法中 goroutine 内部）

- [ ] **Step 1: 修改看门狗 Start 方法 — 将退出事件从独立 goroutine 改为 defer**

文件: `watch_dog_commons_impl.go:81-105`（`Start` 方法中 goroutine 的前半部分，从 `go func()` 到 `time.Sleep(needSleep)`）

```go
	x.isRunning.Store(true)
	go func() {

		// 已经刷新成功多少次了
		refreshSuccessCount := 0
		// 统计连续多少次发生错误了
		continueErrorCount := 0

		// 退出的时候给一个信号，使用 defer 确保在 goroutine 退出时才触发，且能捕获到最终的计数值
		defer func() {
			exitAction := events.NewAction(ActionWatchDogExit).
				AddPayload(PayloadRefreshSuccessCount, refreshSuccessCount).
				AddPayload(PayloadContinueErrorCount, continueErrorCount)
			x.e.Fork().AddAction(exitAction).Publish(context.Background())
		}()

		// 先休眠一下，再死循环刷新
		// 这是针对锁定时间比较短的锁的一个优化，当狗狗休眠结束锁已经被释放掉了，而狗狗也已经被标记为退出状态
		// 能够避免一次无效的刷新，也能够避免因为自身续租而导致的miss率
		// 而对于持有时间比较长的锁来说，也不差这么点时间
		// 时间不要太长，避免协程泄露，1秒封顶
		needSleep := x.storageLock.options.LeaseRefreshInterval
		if needSleep > time.Second {
			needSleep = time.Second
		}
		time.Sleep(needSleep)
```

- [ ] **Step 2: 验证编译通过**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go build ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "error" or "cannot"

- [ ] **Step 3: 质量门禁检查**
Run: `cd /home/cc11001100/github/storage-lock/go-storage-lock && go vet ./... && go test ./...`
Expected:
  - Exit code: 0
  - Output does NOT contain: "FAIL" or "panic"

- [ ] **Step 4: 提交**
Run: `git add watch_dog_commons_impl.go && git commit -m "fix(watchdog): use defer for exit event instead of immediate goroutine

The exit event goroutine fired immediately at startup and captured
refreshSuccessCount/continueErrorCount at creation time (both 0).
Changed to defer so the event fires on goroutine exit with final counts."`

---

## Self-Review Results

**Plan Type:** Bug Fix

| # | Check | Result | Action Taken |
|---|-------|--------|-------------|
| 1 | Goal + Type + Scope + Risk? | PASS | - |
| 2 | Dependencies? | PASS | All tasks independent |
| 3 | Each Task has 3-8 Steps? | PASS | Each has 4 steps |
| 4 | No TBD/TODO/vague descriptions? | PASS | - |
| 5 | Cross-task consistency? | PASS | No shared types |
| 6 | File save location correct? | PASS | docs/superpowers/plans/ |
| 7 | Header contains Symptom + Root Cause + Impact? | PASS | - |
| 8 | Root Cause precise to function+line? | PASS | All three pinpointed |
| 9 | Minimal fixes (no over-modification)? | PASS | Each fix is 1-3 lines |
| 10 | No "incidental improvements"? | PASS | - |
| 11 | Each Task has quality gate? | PASS | Build + vet + test |
| 12 | No "顺便优化"? | PASS | - |

**Status:** ALL PASS

---

## Execution Selection

**Tasks:** 3
**Dependencies:** None (all independent)
**User Preference:** none
**Decision:** Inline (3 independent tiny tasks, each touching 1 file, 1-3 lines changed)
**Reasoning:** Each task is a minimal 1-3 line fix in a single file, inline execution is more efficient than subagent overhead

⏹️ Phase 4 Complete: Proceeding with inline execution
