package ovsdb

import (
	"testing"
)

func TestClient_Healthy_NotInitialised(t *testing.T) {
	var c *Client
	if err := c.Healthy(); err == nil {
		t.Error("expected error for nil receiver")
	}
	if err := (&Client{}).Healthy(); err == nil {
		t.Error("expected error for uninitialised client (nil inner)")
	}
}
