package main

import "testing"

func TestResolveJobParentIDsPrefersRequestOverrides(t *testing.T) {
	ids, scope, err := resolveJobParentIDs([]string{"cfg-1", "cfg-2"}, jobRequest{
		ParentID:  "single",
		ParentIDs: []string{"bulk-a", "bulk-a"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != "partial" {
		t.Fatalf("expected partial scope, got %s", scope)
	}
	if len(ids) != 2 || ids[0] != "bulk-a" || ids[1] != "single" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestResolveJobParentIDsFallsBackToConfig(t *testing.T) {
	ids, scope, err := resolveJobParentIDs([]string{"cfg-1", "cfg-2"}, jobRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != "full" {
		t.Fatalf("expected full scope, got %s", scope)
	}
	if len(ids) != 2 || ids[0] != "cfg-1" || ids[1] != "cfg-2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestResolveJobParentIDsRequiresSource(t *testing.T) {
	_, _, err := resolveJobParentIDs(nil, jobRequest{})
	if err == nil {
		t.Fatalf("expected error when no parent ids in config and request")
	}
}
