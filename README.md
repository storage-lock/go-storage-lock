# Storage Lock

## 这是什么

`go-storage-lock` 抽象了一套建立在存储介质之上的分布式锁模型。正确性依赖底层 Storage 对同一 `lockId` 提供**线性一致的 CAS**，并提供可靠、单调且跨节点可协调的时间源；Watch/通知只用于优化等待，不是互斥性前提。

## 已具备能力

- CAS + 租约的独占锁，支持同一 owner 重入。
- `TryLock` 非阻塞获取；`Renew`、`TTL`、`Inspect` 状态观测。
- `WithLock` 自动获取、回调执行和清理；`StorageLockWithMetadata` 将 string map 扩展字段随锁记录持久化。
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
    // 在与资源写入相同的事务/原子操作中拒绝旧 token。
    return resource.WriteIfFenceIsCurrent(ctx, "account:42", token, payload)
})
```

`WriteIfFenceIsCurrent` 表示资源侧原子操作：仅当 `token` 大于该资源已接受的最大 token 时，更新资源并记录新 token。若使用相同 token 执行多次写入，还需由业务提供幂等/顺序规则。不要先单独读取 token 再分开写业务数据；这会留下竞态窗口。

恰好到达 `LeaseExpireTime` 时租约已经失效。关闭看门狗后，业务必须在租约到期前调用 `Renew`；过期租约不能续活，只能重新获取。

## 能力边界

`LockInformation` 是单 key 的租约记录：独占锁的 `LockCount` 表示同一 owner 的重入深度，`Extra` 是由持有者随锁记录更新的可选业务元数据，不是独立数据库或高并发共享 map。元数据只能随锁的 CAS 更新，不应被用作授权或 fencing 校验的替代。因而：

- 核心不再提供 `Semaphore` 和 `Barrier`。单条锁记录只有一个 owner 和重入计数，表达不了许可槽位、参与者集合和栅栏代际。固定槽位信号量使用 `go-storage-lock-coordination`；倒计时闩、条件变量、栅栏和限流使用 `go-storage-coordination`。
- 多 key 的全有全无只在存储声明 `CapabilityAtomicBatch` 并实现 `CompareAndSwapBatch` 时可用，`MultiLock` 会在同一次提交里获取或释放。没有这个能力的存储会在创建时被拒绝；顺序逐把加锁不再对外伪装成原子。
- 不实现 RedLock。多个独立后端的 quorum 成功无法在分区、异步复制、进程暂停和时钟/延迟不确定时证明线性一致 ownership。优先使用单一强一致后端或共识租约。
- 物理删除墓碑会破坏 fencing 历史；运维工具不应把 `DeleteWithVersion` 当普通强制解锁。

需要固定 slot 信号量、租约选主或受限有序组合锁时，使用同一产品树下的 `go-storage-lock-coordination` 模块。Semaphore 的每个 permit 和 Leader 的每次 campaign 都是独立租约句柄，必须使用 fencing token 保护业务资源。

## 重要部署约束

`CapabilityCAS` 必须代表同一 `lockId` 上真正线性一致的条件写；时间源必须可靠、单调，并在参与节点间协调。对象存储等没有可信服务端时间的介质，需要注入经过验证的 `TimeProvider`，但这**不能弥补条件写不具备线性一致性**的问题。Storage 的 capability 声明是接口自述，不是证明或契约测试。

发布与部署边界：

- 本仓核心 CI 验证锁库构建、静态检查、竞态测试；锁语义套件对列出的本地后端验证锁行为。它们不证明特定线上集群的复制、故障切换、持久化和时钟配置满足前提。
- memory 仅在单进程内协调；filesystem/嵌入式数据库仅在其文档保证的本机共享文件/进程模型下使用。不能仅凭同一 API 或能力标志推导跨主机安全。
- Redis、SQL/NoSQL、etcd、ZooKeeper、对象存储及其他服务端后端必须逐实现、逐部署拓扑确认 CAS 原子性、故障切换行为和时间源。未运行其服务端语义测试的组合应视为未验证，不在本库承诺生产安全。
- 不要用多个独立 Redis/对象存储副本的 quorum 或 RedLock 绕过上述条件。锁服务失联时应按不可用处理，不得把缓存/异步副本的旧状态当成授权。
- 即使锁正确，租约过期后的旧进程仍可能继续执行；业务资源必须原子校验 fencing token。看门狗取消 context 只是通知，不会撤销已发出的写入。

## 安装

```bash
go get github.com/storage-lock/go-storage-lock
```

需要一个后端时，先选 `go-memory-storage`（仅单进程）或某个 `go-*-storage`；`go-*-locks` 把
"构造 Storage"这一步也封装掉了：

```go
package main

import (
    "context"
    "fmt"

    "github.com/storage-lock/go-memory-storage"
    "github.com/storage-lock/go-storage-lock"
)

func main() {
    ctx := context.Background()
    store := memory_storage.NewMemoryStorage()          // 换成任何满足 CAS 前提的存储
    lock, err := storage_lock.NewStorageLock(store, "order:42")
    if err != nil {
        panic(err)
    }

    owner := lock.NewOwnerId()                           // 这个进程的身份
    if err := lock.Lock(ctx, owner); err != nil {        // 阻塞直到拿到或 ctx 取消
        panic(err)
    }
    defer lock.UnLock(ctx, owner)

    // 这里读写受保护资源；需要跨进程安全时，用 fencing token 原子校验。
}
```

## 相关模块

| 模块 | 用途 |
|------|------|
| `go-storage-lock-factory` | 按 Storage 构造锁的门面、重入锁与分布式公平锁 |
| `go-storage-lock-coordination` | 固定槽位信号量、租约选主、受限有序组合锁 |
| `go-storage-coordination` | 倒计时闩、条件变量、栅栏、限流 |
| `go-storage-lock-prometheus` / `-otel` / `-auditlog` / `-metric` | 监控、追踪、审计与本地计数 |
| `storage-lock-cli` / `go-storage-lock-skills` | 命令行工具与 Agent 协作命令 |
| `storage-lock-http-api` / `storage-lock-grpc-api` / `storage-lock-ui` | 把锁暴露成服务与可视化控制台 |

## 行为保证与不保证

- **保证**：同一 lockId 上，只要底层存储对单 key 提供线性一致 CAS、时间源可靠，任意数量进程
  中同一时刻只有一个 owner 持有独占锁；释放后保留版本递增的墓碑，fencing token 不会回退。
- **保证**：公平模式（`Fair` / `FairLock`）的排队顺序写在锁记录里，跨进程生效；等待者崩溃的
  过期登记会被其他实例清理，不会永久堵住队列。
- **不保证**：跨 key 的全有全无 —— 只有声明 `CapabilityAtomicBatch` 的存储才可用 `MultiLock`，
  否则创建时即被拒绝，不会退化成逐把加锁再宣称原子。
- **不保证**：锁正确就等于业务安全。租约过期后的旧进程仍可能在执行，资源侧必须原子校验
  fencing token；看门狗取消 context 只是通知，撤销不了已经发出的写入。
