package file_system_storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang-infrastructure/go-iterator"
	"github.com/storage-lock/go-storage-lock/pkg/storage"
	"github.com/storage-lock/go-storage-lock/pkg/storage_lock"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FileSystemStorage 基于文件系统来存储锁，可以用作同一台机器上的不同进程之间互相协同工作，优势是兼容性好，只要有文件系统能读写文件就能使用这种锁
type FileSystemStorage struct {
	workspace string
}

var _ storage.Storage = &FileSystemStorage{}

// NewFileSystemStorage 基于文件系统存储锁的时候必须指定一个存储锁的工作目录
func NewFileSystemStorage(workspace string) *FileSystemStorage {
	return &FileSystemStorage{
		workspace: workspace,
	}
}

const StorageName = "file-system-storage"

func (x *FileSystemStorage) GetName() string {
	return StorageName
}

func (x *FileSystemStorage) Init(ctx context.Context) error {
	stat, err := os.Stat(x.workspace)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.MkdirAll(x.workspace, os.ModePerm)
		}
		return err
	} else if !stat.IsDir() {
		return fmt.Errorf("path %s is not directory", x.workspace)
	}

	// TODO 检查目录下是否有写权限

	return nil
}

const (

	// LockDirectoryPrefix 存放锁的目录名必须有这个前缀
	LockDirectoryPrefix = "storage-lock-"

	// LockDirectorySuffix 存放锁的目录名必须有这个后缀
	LockDirectorySuffix = ".lock"
)

// IsLockDirectory 判断这个路径是否是一个存放锁的路径
func (x *FileSystemStorage) IsLockDirectory(path string) bool {

	// 名称必须符合标准，有特定的前缀和后缀
	if !strings.HasPrefix(path, LockDirectoryPrefix) || !strings.HasSuffix(path, LockDirectorySuffix) {
		return false
	}

	// 必须是一个目录
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat.IsDir()
}

// BuildLockDirectory 锁的多个版本是放在同一个文件夹中的
func (x *FileSystemStorage) BuildLockDirectory(lockId string) string {
	return filepath.Join(x.workspace, LockDirectoryPrefix, lockId, LockDirectorySuffix)
}

// BuildLockVersionFilePath 根据锁的ID和版本构建对应的锁文件地址
func (x *FileSystemStorage) BuildLockVersionFilePath(lockId string, version storage.Version) string {
	return filepath.Join(x.BuildLockDirectory(lockId), strconv.Itoa(int(version)))
}

// EnsureDirectoryExists 确保给定的路径存在并且是一个目录
func (x *FileSystemStorage) EnsureDirectoryExists(directory string) error {
	_ = os.MkdirAll(directory, os.ModePerm)
	stat, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("path %s is not directory", directory)
	}
	return nil
}

func (x *FileSystemStorage) UpdateWithVersion(ctx context.Context, lockId string, exceptedVersion, newVersion storage.Version, lockInformation *storage.LockInformation) error {

	// 确认目前最新的版本是期望的版本
	lockInformation, err := x.ReadLockLatestVersionInformation(x.BuildLockDirectory(lockId))
	if err != nil {
		return err
	}
	if lockInformation == nil || lockInformation.Version != exceptedVersion {
		return storage_lock.ErrLockFailed
	}

	// 然后开始尝试更新锁
	lockVersionPath := x.BuildLockVersionFilePath(lockId, newVersion)
	lockVersionFile, err := os.Create(lockVersionPath)
	if err != nil {
		// TODO 文件创建失败，意味着锁也获取失败了
		return storage_lock.ErrLockFailed
	}
	// 获取到版本锁了，则将其内容写入到文件中
	_, err = lockVersionFile.WriteString(lockInformation.ToJsonString())
	if err != nil {
		return storage_lock.ErrLockFailed
	}

	return nil
}

func (x *FileSystemStorage) InsertWithVersion(ctx context.Context, lockId string, version storage.Version, lockInformation *storage.LockInformation) error {
	lockDirectory := x.BuildLockDirectory(lockId)
	err := x.EnsureDirectoryExists(lockDirectory)
	if err != nil {
		return err
	}
	lockVersionPath := x.BuildLockVersionFilePath(lockId, version)
	lockVersionFile, err := os.Create(lockVersionPath)
	if err != nil {
		return err
	}
	_, err = lockVersionFile.WriteString(lockInformation.ToJsonString())
	if err != nil {
		return err
	}
	return nil
}

func (x *FileSystemStorage) DeleteWithVersion(ctx context.Context, lockId string, exceptedVersion storage.Version, lockInformation *storage.LockInformation) error {
	lockVersionFilePath := x.BuildLockVersionFilePath(lockId, exceptedVersion)
	path, err := x.ReadLockInformationFromPath(lockVersionFilePath)
	if err != nil {
		return err
	}
	if path.ToJsonString() != lockInformation.ToJsonString() {
		return storage_lock.ErrUnlockFailed
	}
	// 删除锁就是把整个文件夹都删除掉
	return os.RemoveAll(x.BuildLockDirectory(lockId))
}

func (x *FileSystemStorage) Get(ctx context.Context, lockId string) (string, error) {
	// TODO 效率优化
	information, err := x.ReadLockLatestVersionInformation(lockId)
	if err != nil || information == nil {
		return "", err
	}
	return information.ToJsonString(), nil
}

func (x *FileSystemStorage) GetTime(ctx context.Context) (time.Time, error) {
	// 因为是在同一个文件系统，所以这里简单的取系统时间了
	// TODO 实际上对于共享文件系统之类的而言并不一定真的是同一个系统，这样子获取是有概率出问题的
	return time.Now(), nil
}

func (x *FileSystemStorage) Close(ctx context.Context) error {
	// TODO 清理过期的锁文件，但又不能误删最近的锁文件
	return nil
}

func (x *FileSystemStorage) List(ctx context.Context) (iterator.Iterator[*storage.LockInformation], error) {
	dirEntrySlice, err := os.ReadDir(x.workspace)
	if err != nil {
		return nil, err
	}
	lockInformationSlice := make([]*storage.LockInformation, 0)
	for _, dirEntry := range dirEntrySlice {
		if strings.HasPrefix(dirEntry.Name(), LockDirectoryPrefix) {
			lockId := x.ExtractLockIdFromLockDirectoryName(dirEntry.Name())
			lockInformation, err := x.ReadLockLatestVersionInformation(lockId)
			if err != nil {
				return nil, err
			}
			lockInformationSlice = append(lockInformationSlice, lockInformation)
		}
	}
	return iterator.NewSliceIterator(lockInformationSlice), nil
}

// ExtractLockIdFromLockDirectoryName 从存放锁的目录名中抽取锁的ID
func (x *FileSystemStorage) ExtractLockIdFromLockDirectoryName(lockDirectoryName string) string {
	if !x.IsLockDirectory(lockDirectoryName) {
		return ""
	}
	// 把前缀去除
	lockDirectoryName = lockDirectoryName[0 : len(LockDirectoryPrefix)+1]
	// 把后缀去除
	lockDirectoryName = lockDirectoryName[0 : len(lockDirectoryName)-len(LockDirectorySuffix)]
	return lockDirectoryName
}

// ReadLockLatestVersionInformation 从锁文件夹中读取最新版本的信息
func (x *FileSystemStorage) ReadLockLatestVersionInformation(lockId string) (*storage.LockInformation, error) {
	directory := x.BuildLockDirectory(lockId)
	dirEntrySlice, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(dirEntrySlice) == 0 {
		return nil, nil
	}

	// TODO 提高排序效率
	sort.Slice(dirEntrySlice, func(i, j int) bool {
		versionI, err := strconv.Atoi(dirEntrySlice[i].Name())
		if err != nil {
			return true
		}
		versionJ, err := strconv.Atoi(dirEntrySlice[j].Name())
		if err != nil {
			return false
		}
		return versionI < versionJ
	})

	lockVersionPath := filepath.Join(directory, dirEntrySlice[len(dirEntrySlice)-1].Name())
	return x.ReadLockInformationFromPath(lockVersionPath)
}

// ReadLockInformationFromPath 从文件中读取锁信息
func (x *FileSystemStorage) ReadLockInformationFromPath(lockVersionPath string) (*storage.LockInformation, error) {
	fileBytes, err := os.ReadFile(lockVersionPath)
	if err != nil {
		return nil, err
	}
	return storage.FromJsonString(string(fileBytes))
}
