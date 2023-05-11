package redis_storage

//import (
//	"context"
//	"fmt"
//	"github.com/redis/go-redis/v9"
//	"time"
//)
//
//// ------------------------------------------------ ---------------------------------------------------------------------
//
//// RedisConfigurationConnectionGetter 根据Redis的配置获取连接
//type RedisConfigurationConnectionGetter struct {
//
//	// Redis服务器的地址，比如192.168.1.1
//	Address string
//
//	// Redis的密码，不推荐使用空密码
//	Passwd string
//
//	// 选择哪个DB
//	DB int
//
//	client *redis.Client
//}
//
//var _ ConnectionGetter[*redis.Client] = &RedisConfigurationConnectionGetter{}
//
//func NewRedisConfigurationConnectionGetter(address, passwd string, db ...int) *RedisConfigurationConnectionGetter {
//	if len(db) == 0 {
//		db = append(db, 0)
//	}
//	return &RedisConfigurationConnectionGetter{
//		Address: address,
//		Passwd:  passwd,
//		DB:      db[0],
//	}
//}
//
//func (x *RedisConfigurationConnectionGetter) Get(ctx context.Context) (*redis.Client, error) {
//	if x.client == nil {
//		x.client = redis.NewClient(&redis.Options{
//			Addr:     x.Address,
//			Password: x.Passwd,
//			DB:       x.DB,
//		})
//	}
//	return x.client, nil
//}
//
//// ------------------------------------------------ ---------------------------------------------------------------------
//
//type RedisStorage struct {
//	connectionGetter ConnectionGetter[*redis.Client]
//}
//
//var _ Storage = &RedisStorage{}
//
//func NewRedisStorage(connectionGetter ConnectionGetter[*redis.Client]) *RedisStorage {
//	return &RedisStorage{
//		connectionGetter: connectionGetter,
//	}
//}
//
//func (x *RedisStorage) Init(ctx context.Context) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *RedisStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion Version, lockInformation *LockInformation) error {
//
//}
//
//func (x *RedisStorage) InsertWithVersion(ctx context.Context, lockId string, version Version, lockInformation *LockInformation) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *RedisStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion Version, lockInformation *LockInformation) error {
//	client, err := x.connectionGetter.Get(ctx)
//	if err != nil {
//		return err
//	}
//	client.Del(ctx, x.buildLockKey(lockId))
//}
//
//func (x *RedisStorage) Get(ctx context.Context, lockId string) (string, error) {
//	var zero time.Time
//	client, err := x.connectionGetter.Get(ctx)
//	if err != nil {
//		return "", err
//	}
//	get := client.Get(ctx, lockId)
//	get.
//
//}
//
//// GetTime 获取Redis服务器的时间
//func (x *RedisStorage) GetTime(ctx context.Context) (time.Time, error) {
//	var zero time.Time
//	client, err := x.connectionGetter.Get(ctx)
//	if err != nil {
//		return zero, err
//	}
//	cmd := client.Time(ctx)
//	if cmd.Err() != nil {
//		return zero, cmd.Err()
//	}
//	return cmd.Val(), nil
//}
//
//// 根据锁的ID构建
//func (x *RedisStorage) buildLockKey(lockId string, version ...int) string {
//	if len(version) == 0 {
//		return "storage-lock:redis:" + lockId
//	} else {
//		return fmt.Sprintf("storage-lock:redis:%s:%d", lockId, version)
//	}
//}
//
//// ------------------------------------------------ ---------------------------------------------------------------------
