package metric

// 可观测性是基于事件的

// TODO 2023-8-4 01:22:10 思考如何进行性能统计啥的
type Metric struct {
	LockMetric    *LockMetric
	StorageMetric *StorageMetric
}
