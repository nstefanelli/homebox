package main

import (
	"context"
	"testing"
	"time"
)

func TestNewTaskRetainsName(t *testing.T) {
	task := NewTask("daily-cleanup", time.Hour, func(context.Context) {})
	if task.Name() != "daily-cleanup" {
		t.Fatalf("Name() = %q, want %q", task.Name(), "daily-cleanup")
	}
}
