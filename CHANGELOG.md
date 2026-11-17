# Changelog

## v0.1.0

### 破坏性变更

- `FairLock.Lock / TryLock / UnLock` 增加 `ownerId` 参数。旧实现把
  `fairlock-<lockId>` 当 owner，同一 lockId 上所有客户端共用一个 owner，
  导致两个进程可以同时持锁，且一方 `UnLock` 会连带释放另一方持有的锁。
  现在每次调用由调用方传入自己的身份。
- `NewMultiLock(ownerId string, locks ...*StorageLock)` 增加 `ownerId` 参数。
- `FairStorageLock` 改名为 `LocalOrderedLock`（旧名保留为类型别名）。它只在
  本进程内排队，不提供跨进程公平；需要跨进程顺序时用 `FairLock` 或
  `StorageLockOptions.Fair`。

### 修复

- **`FairLock` 互斥失效（v0.0.6 / v0.0.7 均受影响）**：见上。已补三组回归测试
  （双实例互斥、释放作用域、并发临界区），并验证这些测试对旧实现会失败。
- 公平排队登记增加 `Until` 截止时间，等待者崩溃后来的实例可以清理过期登记，
  不会永久堵住队列。
- 看门狗确认持有权丢失时取消业务 context（`ErrLockLost`），并区分"租约到期"
  与"被他人接管"，不再对正常释放误报丢失。
- 续租间隔与租约过期之间强制保留安全余量，避免抖动导致租约被提前抢占。
- `stopWatchDog` / `signalOwnershipLost` 的互斥死锁（持有 `watchDogMu` 等待退出
  与看门狗等待该锁）。
- `Stop` 无法打断卡在存储 RPC 里的 goroutine 导致的 liveness 泄漏。
- `SetEvent` 替换事件源的数据竞态。

### 新增

- `CapabilityAtomicBatch` / `AtomicBatchStorage`：跨 key 全有全无锁必须由存储
  在同一次线性一致提交里完成。没有该能力的存储会被 `NewMultiLock` 拒绝，
  不再退化成逐把加锁再宣称原子。
- `StorageLockWithMetadata`：string map 扩展字段随锁记录持久化。
- `LockMetrics`：进程内事件计数，可交给 Prometheus / OpenTelemetry 导出。
- 公平队列跨进程生效，不依赖进程内内存队列。

### 依赖

- 升级 `go-storage` 到 v0.1.0（新增 `LockInformation.Extra`、`LockWaiter.Until`、
  `CapabilityAtomicBatch` / `CASOp` / `AtomicBatchStorage`）。

## v0.0.7

- 支持无服务端时钟/无原子删除的存储介质接入。
- 暴露 fencing token；释放后保留墓碑记录，版本继续递增。
- `TryUpgrade` / `Downgrade` 读写锁升降级；写意图阻止写饥饿。
- 修复看门狗雪崩、无效防御、回滚墓碑不一致等理论漏洞。

## v0.0.6

- 订阅通知接通存储原生 Watch，具备跨实例可见性。
- 修复 `UpdateWithVersion` 在记录不存在时误报不可重试错误。
