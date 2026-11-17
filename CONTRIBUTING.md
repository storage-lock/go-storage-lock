# Contributing

## 开发环境

需要 Go 1.23 或更新版本。仓库根目录的 `go.work` 把全部模块串成一个工作区：

```bash
git clone https://github.com/storage-lock/go-storage-lock.git
cd go-storage-lock
go build ./...
go test ./...
```

## 提交前自检

```bash
go build ./... && go vet ./... && go test -race ./...
```

新增或修改锁语义时，**必须**同时补测试，并确认该测试在旧实现上会失败
（否则它不能防止回归）。公平锁、租约、看门狗这类状态机尤其如此。

## 能力声明纪律

存储实现声明 `CapabilityCAS` / `CapabilityReliableTime` / `CapabilityAtomicBatch`
之前，先确认实现真的是线性一致条件写 / 可信服务端时钟 / 单次原子提交。
能力标志是接口自述，不是证明——虚标会让上层误以为可以安全建锁。
不确定时宁可少声明，让 `NewStorageLock` 拒绝，也不要给出会误判互斥的后端。

## 提交信息

用 `fix:` / `feat:` / `build:` / `docs:` / `chore:` 前缀。修复正确性问题时，
在正文里写清触发条件、影响范围，以及新测试如何覆盖它。
