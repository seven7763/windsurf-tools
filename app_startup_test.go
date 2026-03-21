package main

import (
	"path/filepath"
	"testing"
	"windsurf-tools-wails/backend/store"
)

func TestNewStoreInPathsCreatesAndLoads(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "cfg")
	s, err := store.NewStoreInPaths(sub)
	if err != nil {
		t.Fatal(err)
	}
	if s == nil {
		t.Fatal("nil store")
	}
}
