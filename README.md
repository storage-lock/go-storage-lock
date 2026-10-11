# Storage Lock

## 这是什么

`go-storage-lock` 抽象了一套建立在存储介质之上的分布式锁模型。正确性依赖底层 Storage 对同一 `lockId` 提供**线性一致的 CAS**，并提供可靠、单调且跨节点可协调的时间源；Watch/通知只用于优化等待，不是互斥性前提。

## 已具备能力

- CAS + 租约的独占锁，支持同一 owner 重入。
- `TryLock` 非阻塞获取；`Renew`、`TTL`、`Inspect` 状态观测。
- `WithLock` 自动获取、回调执行和清理。
- 自动续租看门狗，也可关闭后手动续租。
- 读写锁、状态订阅和 fencing token。
- `TryUpgrade` 把唯一读者的读锁升级为独占写锁；还有其他读者时失败并保持原读锁。`Downgrade` 把一层独占写锁降为共享读锁。
- 写者等待期间登记写意图，阻止新读者进入，避免写饥饿。`Inspect` 和 `Waiters` 返回等待队列、写意图和已等待时长。
- `Fair: true` 把等待顺序变成获取条件。等待者按登记顺序写在锁记录里，只有排在最前的写者能拿走空闲或过期的锁；记录上有排队时，没开这个选项的客户端也不能插队。这个顺序跨进程有效，不依赖调用方是否在同一个进程。排队的人崩溃或取消后，下一次获取会把自己从队列里去掉，后面的人继续按顺序接手。
- `WithLock` 与 `BeginLease` 在看门狗确认持有权丢失时取消业务 context（`ErrLockLost`）。取消不能追回已发出的写入，资源侧仍要校验 fencing token。
- 释放后保留墓碑记录（`LockCount == 0`），版本继续递增，避免 fencing token 因删除而 ABA 回退。

典型用法：

```go
err := lock.WithLock(ctx, func(ctx context.Context, token storage.Version) error {
    // 受保护资源必须原子校验 token，拒绝旧 token 的写入。
    return nil
})
```

恰好到达 `LeaseExpireTime` 时租约已经失效。关闭看门狗后，业务必须在租约到期前调用 `Renew`；过期租约不能续活，只能重新获取。

## 能力边界

`LockInformation` 是单 key、单 owner 的租约记录：`LockCount` 表示同一 owner 的重入深度，不是任意参与者可以安全递减的分布式计数器。因而：

- 核心不再提供 `Semaphore` 和 `Barrier`。单条锁记录只有一个 owner 和重入计数，表达不了许可槽位、参与者集合和栅栏代际。固定槽位信号量使用 `go-storage-lock-coordination`；倒计时闩、条件变量、栅栏和限流使用 `go-storage-coordination`。
- 多 key 的严格全有全无锁需要事务或多 key CAS；顺序组合只能提供非原子 best-effort 语义。
- 不实现 RedLock。多个独立后端的 quorum 成功无法在分区、异步复制、进程暂停和时钟/延迟不确定时证明线性一致 ownership。优先使用单一强一致后端或共识租约。
- 物理删除墓碑会破坏 fencing 历史；运维工具不应把 `DeleteWithVersion` 当普通强制解锁。

需要固定 slot 信号量、租约选主或受限有序组合锁时，使用同一产品树下的 `go-storage-lock-coordination` 模块。Semaphore 的每个 permit 和 Leader 的每次 campaign 都是独立租约句柄，必须使用 fencing token 保护业务资源。

## 重要部署约束

`CapabilityCAS` 必须代表真正线性一致的条件写，而不是最终一致副本上的本地检查。Storage 的 capability 声明不能替代后端契约测试；对象存储、异步副本和仅进程内 memory 不应被宣传为跨进程生产锁。时间源也必须经过部署验证，不能用无约束的本地时钟替代可靠时间。

## 安装

```bash
go get github.com/storage-lock/go-storage-lock
```
