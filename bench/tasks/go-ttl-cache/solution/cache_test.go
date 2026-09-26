package cache

import (
	"testing"
	"time"
)

func TestBasic(t *testing.T) {
	c := New(1, time.Hour, nil)
	c.Set("a", "1")
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatal("get")
	}
}
