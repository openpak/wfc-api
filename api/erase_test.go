package api

import (
	"context"
	"testing"
)

type fakeLinks map[string][]int64

func (f fakeLinks) UserIDs(_ context.Context, a string) ([]int64, error) { return f[a], nil }

type fakeDB struct{ users map[int64]int }

func (f *fakeDB) EraseUsers(_ context.Context, ids []int64) (int, error) {
	n := 0
	for _, id := range ids {
		n += f.users[id]
		delete(f.users, id)
	}
	return n, nil
}

func TestEraseAccountOnlyThatAccount(t *testing.T) {
	links := fakeLinks{"a": {1, 2}, "b": {3}}
	db := &fakeDB{users: map[int64]int{1: 4, 2: 1, 3: 9}}
	n, err := eraseAccount(context.Background(), "a", links, db)
	if err != nil || n != 5 {
		t.Fatalf("%d %v", n, err)
	}
	if db.users[3] != 9 {
		t.Fatal("other account touched")
	}
	if n, err := eraseAccount(context.Background(), "a", links, db); err != nil || n != 0 {
		t.Fatalf("second run %d %v", n, err)
	}
}
