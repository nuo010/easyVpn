package server

import "testing"

func TestIPAllocatorAllocateSequential(t *testing.T) {
	allocator, err := NewIPAllocator("10.200.0.1/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	first, err := allocator.Allocate("peer-a", "")
	if err != nil {
		t.Fatalf("allocate first: %v", err)
	}
	second, err := allocator.Allocate("peer-b", "")
	if err != nil {
		t.Fatalf("allocate second: %v", err)
	}

	if first != "10.200.0.2/24" {
		t.Fatalf("unexpected first allocation: %s", first)
	}
	if second != "10.200.0.3/24" {
		t.Fatalf("unexpected second allocation: %s", second)
	}
}

func TestIPAllocatorStaticReserveAndRelease(t *testing.T) {
	allocator, err := NewIPAllocator("10.200.0.1/24")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assigned, err := allocator.Allocate("peer-a", "10.200.0.50/24")
	if err != nil {
		t.Fatalf("allocate static: %v", err)
	}
	if assigned != "10.200.0.50/24" {
		t.Fatalf("unexpected static allocation: %s", assigned)
	}

	if _, err := allocator.Allocate("peer-b", "10.200.0.50/24"); err == nil {
		t.Fatalf("expected duplicate static allocation to fail")
	}

	allocator.Release("peer-a", assigned)
	if _, err := allocator.Allocate("peer-b", "10.200.0.50/24"); err != nil {
		t.Fatalf("expected released static allocation to be reusable: %v", err)
	}
}
