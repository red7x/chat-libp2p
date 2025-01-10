package database

import (
	"fmt"
	"os"
	"time"

	"go.etcd.io/bbolt"
)

type Bolt struct {
	db *bbolt.DB
}

func NewBolt(path string) (bolt *Bolt, err error) {
	db, err := bbolt.Open(path, os.ModePerm, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		err = fmt.Errorf("failed to create bolt db: %v", err)
		return
	}
	bolt = &Bolt{
		db: db,
	}
	return
}

func (b *Bolt) DB() *bbolt.DB {
	return b.db
}

func (b *Bolt) Close() (err error) {
	return b.db.Close()
}
