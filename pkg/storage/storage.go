package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"go.etcd.io/bbolt"
)

func initStorageDir(dir string) error {
	if stat, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("could not create directory: %w", err)
		}
	} else if !stat.IsDir() {
		return fmt.Errorf("not a valid directory: %w", err)
	}
	return nil
}
func New(storageDir, bucket string) (*BoltStorage, error) {
	if err := initStorageDir(storageDir); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(storageDir, "dids.db")
	db, err := bbolt.Open(dbPath, 0600, bbolt.DefaultOptions)
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucket))
		return err
	}); err != nil {
		return nil, fmt.Errorf("could not create bucket: %w", err)
	}

	return &BoltStorage{
		db:     db,
		bucket: []byte(bucket),
	}, nil
}

type BoltStorage struct {
	bucket []byte
	db     *bbolt.DB
}

func (s *BoltStorage) Set(id string, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(s.bucket).Put([]byte(id), value)
	})
}

func (s *BoltStorage) Get(id string) ([]byte, error) {
	var data []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		data = tx.Bucket(s.bucket).Get([]byte(id))
		return nil
	})
	return data, err
}

func (s *BoltStorage) Delete(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(s.bucket).Delete([]byte(id))
	})
}
