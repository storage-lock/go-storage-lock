package storage_lock

//import (
//	"context"
//	"sync/atomic"
//	"time"
//)
//
//// 存储锁的表是否已经存在了
//var storageTableCreated = atomic.Bool{}
//
//type MySQLStorage struct {
//	tableName string
//}
//
//var _ Storage = &MySQLStorage{}
//
//func (x *MySQLStorage) UpdateWithVersion(ctx context.Context, lockId, exceptedValue, newValue string) error {
//	updateSql := `UPDATE %s SET value = $1 WHERE key = $2 AND value = $3 `
//}
//
//func (x *MySQLStorage) DeleteWithVersion(ctx context.Context, lockId, exceptedValue string) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *MySQLStorage) Get(ctx context.Context, lockId string) (string, error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (x *MySQLStorage) createStorageTable() {
//
//}
//
//
//
//updateSql := `UPDATE selefra_meta_kv SET value = $1 WHERE key = $2 AND value = $3 `
//rs, err := x.pool.Exec(ctx, updateSql, information.ToJsonString(), lockKey, oldJsonString)
//if err != nil {
//return err
//}
//if rs.RowsAffected() == 0 {
//return ErrLockFailed
//} else {
//return nil
//}
//} else {
//// If a storageLock exists but is not held by itself, check to see if it is an expired storageLock
//if information.LeaseExpireTime.After(time.Now()) {
//// If the storageLock is not expired, it has to be abandoned
//return ErrLockFailed
//}
//// If the storageLock has expired, delete it and try to reacquire it
//dropExpiredLockSql := `DELETE FROM selefra_meta_kv WHERE key = $1 AND value = $2`
//_, err := x.pool.Exec(ctx, dropExpiredLockSql, lockKey, oldJsonString)
//if err != nil {
//return err
//}
//// TODO
//return x.Lock(ctx, lockId, ownerId)
//}
//}
//
//// The storageLock does not exist. Attempt to obtain the storageLock
//lockInformation := &LockInformation{
//OwnerId:   ownerId,
//LockCount: 1,
//// By default, a storageLock is expected to hold for at least ten minutes
//LeaseExpireTime: time.Now().Add(time.Minute * 10),
//}
//sql := `INSERT INTO selefra_meta_kv (
//                             "key",
//                             "value"
//                             ) VALUES ( $1, $2 )`
//exec, err := x.pool.Exec(ctx, sql, lockKey, lockInformation.ToJsonString())
//if err != nil || exec.RowsAffected() != 1 {
//// storageLock failed
//return ErrLockFailed
//}
//
//// storageLock success, run refresh goroutine
//storageLock.Lock()
//defer storageLock.Unlock()
//goroutine := lockRefreshGoroutineMap[lockId]
//if goroutine != nil {
//goroutine.Stop()
//}
//refreshGoroutine := NewLockRefreshGoroutine(x, lockId, ownerId)
//refreshGoroutine.Start()
//lockRefreshGoroutineMap[lockId] = refreshGoroutine
//return nil
//}
//
//// UnLock 尝试释放锁
//func (x *StorageLock) UnLock(ctx context.Context, ownerId ...string) error {
//	if len(ownerId) == 0 {
//		// TODO default take goroutine ID
//		ownerId = append(ownerId, "xxx")
//	}
//	lockInformation, err := x.getLockInformation(ctx)
//	if err != nil {
//		return err
//	}
//	if lockInformation == nil {
//		return ErrLockNotFound
//	}
//	// storageLock exists, check it's owner
//	if lockInformation.OwnerId != ownerId {
//		return ErrLockNotBelongYou
//	}
//	oldJsonString := lockInformation.ToJsonString()
//	// ok, storageLock is mine, storageLock count - 1
//	lockInformation.LockCount--
//	if lockInformation.LockCount > 0 {
//		// It is not released completely, but the count is reduced by 1 and updated back to the database
//		// Is reentrant to acquire the storageLock, increase the number of locks by 1
//		lockInformation.LeaseExpireTime = time.Now().Add(time.Minute * 10)
//		// compare and set
//		updateSql := `UPDATE selefra_meta_kv SET value = $1 WHERE key = $2 AND value = $3 `
//		rs, err := x.pool.Exec(ctx, updateSql, lockInformation.ToJsonString(), lockKey, oldJsonString)
//		if err != nil {
//			return err
//		}
//		if rs.RowsAffected() == 0 {
//			return ErrUnlockFailed
//		} else {
//			return nil
//		}
//	}
//
//	// Once storageLock count is free, it needs to be completely free, which in this case means delete
//	deleteSql := `DELETE FROM selefra_meta_kv WHERE key = $1 AND value = $2`
//	exec, err := x.pool.Exec(ctx, deleteSql, lockKey, oldJsonString)
//	if err != nil {
//		return err
//	}
//	if exec.RowsAffected() == 0 {
//		return ErrUnlockFailed
//	}
//
//	// stop refresh goroutine
//	storageLock.Lock()
//	defer storageLock.Unlock()
//	goroutine := lockRefreshGoroutineMap[lockId]
//	if goroutine != nil {
//		goroutine.Stop()
//	}
//
//	return nil
//}
//
//// 获取之前的锁保存的信息
//func (x *StorageLock) getLockInformation(ctx context.Context) (*LockInformation, error) {
//	lockInformationJsonString, err := x.storage.Get(ctx, x.options.LockId)
//	if err != nil {
//		return nil, err
//	}
//	return FromJsonString(lockInformationJsonString)
//}