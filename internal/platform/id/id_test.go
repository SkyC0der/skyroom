package id

import (
	"strings"
	"testing"
)

func TestNewReturnsUUIDv7(t *testing.T) {
	t.Parallel()
	value, err := New()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(value, "-")
	if len(parts) != 5 || len(value) != 36 {
		t.Fatalf("expected UUID shape, got %q", value)
	}
	if parts[2][0] != '7' {
		t.Fatalf("expected UUIDv7 version nibble, got %q", value)
	}
	if !strings.Contains("89ab", string(parts[3][0])) {
		t.Fatalf("expected RFC 9562 variant, got %q", value)
	}
}
