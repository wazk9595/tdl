package send

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.etcd.io/bbolt"
)

type Store struct {
	db   *bbolt.DB
	mu   sync.Mutex
	task Task
}

func OpenStore(dir string, initial Task) (*Store, error) {
	if initial.Entries == nil {
		initial.Entries = map[string]*Entry{}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := bbolt.Open(filepath.Join(dir, initial.ID+".db"), 0o600, &bbolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("打开任务记录失败（可能已有同一任务正在运行）：%w", err)
	}
	s := &Store{db: db, task: initial}
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("task"))
		if err != nil {
			return err
		}
		if data := bucket.Get([]byte("state")); data != nil {
			if err := json.Unmarshal(data, &s.task); err != nil {
				return err
			}
			sameRoot := s.task.Root == initial.Root || (runtime.GOOS == "windows" && strings.EqualFold(s.task.Root, initial.Root))
			if s.task.Version != 1 || s.task.ID != initial.ID || !sameRoot || s.task.Namespace != initial.Namespace || s.task.AccountID != initial.AccountID || s.task.Destination.PeerID != initial.Destination.PeerID || s.task.Destination.Topic != initial.Destination.Topic {
				return fmt.Errorf("任务记录与当前账号或目标不一致")
			}
			return nil
		}
		data, err := json.Marshal(initial)
		if err != nil {
			return err
		}
		return bucket.Put([]byte("state"), data)
	})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Claim(hash string, id int) error {
	return s.Update(func(t *Task) error {
		for key, entry := range t.Entries {
			if key != hash && entry.MessageID == id {
				return fmt.Errorf("该消息回执已归属其他文件，需人工核对")
			}
		}
		e := t.Entries[hash]
		if e == nil {
			return fmt.Errorf("未知文件")
		}
		e.MessageID = id
		e.Status = Sent
		e.Updated = time.Now()
		return nil
	})
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Snapshot() Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(s.task)
	var copy Task
	_ = json.Unmarshal(data, &copy)
	return copy
}

func (s *Store) Update(fn func(*Task) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.task)
	if err != nil {
		return err
	}
	var next Task
	if err = json.Unmarshal(data, &next); err != nil {
		return err
	}
	if err = fn(&next); err != nil {
		return err
	}
	data, err = json.Marshal(next)
	if err != nil {
		return err
	}
	if err = s.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket([]byte("task")).Put([]byte("state"), data) }); err != nil {
		return err
	}
	s.task = next
	return nil
}

func (s *Store) Entry(hash string) Entry {
	task := s.Snapshot()
	if e := task.Entries[hash]; e != nil {
		return *e
	}
	return Entry{}
}

func (s *Store) Change(hash string, fn func(*Entry)) error {
	return s.Update(func(t *Task) error {
		e := t.Entries[hash]
		if e == nil {
			return fmt.Errorf("任务中不存在文件 %s", hash)
		}
		fn(e)
		e.Updated = time.Now()
		return nil
	})
}
