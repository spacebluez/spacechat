package main

import "testing"

func TestDefaultServerUsesLoopback(t *testing.T) {
	if defaultServer != "ws://127.0.0.1:18081/ws" {
		t.Fatal("source default must use loopback; provide a deployment address at build or runtime")
	}
}
