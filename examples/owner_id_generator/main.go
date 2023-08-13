package main

import (
	"fmt"
	storage_lock "github.com/storage-lock/go-storage-lock"
)

func main() {

	generator := storage_lock.NewOwnerIdGenerator()
	ownerId := generator.GenOwnerId()
	fmt.Println(ownerId)
	// Output:
	// storage-lock-owner-id-DESKTOP-PL5RI7C-72c2cd2add88b29799e2f2646be16131-1-541201dd0eee40bba0f956ddce427f7c

}
