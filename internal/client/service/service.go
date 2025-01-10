package service

import (
	"fmt"

	"go.etcd.io/bbolt"
)

type CtxKey string

const (
	CtxKeyEVMAddr  CtxKey = "evm_addr"
	CtxKeyImageDir CtxKey = "image_dir"
)

type BucketKey []byte

var (
	BucketKeyPeer        BucketKey = []byte("peers")
	BucketKeyImages      BucketKey = []byte("images")
	BucketKeyMsgPending  BucketKey = []byte("msg_pending_")
	BucketKeyMsgReceived BucketKey = []byte("msg_received_")
	BucketKeyDialogue    BucketKey = []byte("dialogue_")
	BucketKeyDestroyMsg  BucketKey = []byte("destory_msg")
)

func (b BucketKey) Suffix(suffix []byte) BucketKey {
	return append(b, suffix...)
}

func (b BucketKey) GetBucket(tx *bbolt.Tx) (bucket *bbolt.Bucket, err error) {
	if b[len(b)-1] == '_' {
		err = fmt.Errorf("bucket key must have suffix")
		return
	}
	bucket = tx.Bucket(b)
	return
}

func (b BucketKey) GetOrNewBucket(tx *bbolt.Tx) (bucket *bbolt.Bucket, err error) {
	bucket, err = b.GetBucket(tx)
	if err != nil {
		return
	} else if bucket == nil {
		bucket, err = tx.CreateBucket(b)
	}
	return
}

func (b BucketKey) TruncateBucket(tx *bbolt.Tx) (err error) {
	tx.DeleteBucket(b)
	_, err = tx.CreateBucket(b)
	return
}
