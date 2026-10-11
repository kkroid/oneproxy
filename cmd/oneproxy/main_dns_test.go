package main

import (
	"os"
	"strings"
	"testing"
)

func TestDNSNoAutomaticRecoveryWiring(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{".SetAllDownCallback(", ".FlushAll(", ".FlushSystemDNS("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("CLI must not wire automatic DNS recovery: %s", forbidden)
		}
	}
}
