package main

import "testing"

func TestCollectParentOverrides(t *testing.T) {
	ids := collectParentOverrides("925", " 100,200,100 ,, ")
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique ids, got %d (%v)", len(ids), ids)
	}
	if ids[0] != "100" || ids[1] != "200" || ids[2] != "925" {
		t.Fatalf("unexpected ids order/content: %v", ids)
	}
}

func TestResolveParentIDsPrefersOverrideAsPartial(t *testing.T) {
	ids, scope, err := resolveParentIDs([]string{"cfg1", "cfg2"}, []string{"override1", "override1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != scopeModePartial {
		t.Fatalf("expected partial scope, got %s", scope)
	}
	if len(ids) != 1 || ids[0] != "override1" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestResolveParentIDsUsesConfigAsFull(t *testing.T) {
	ids, scope, err := resolveParentIDs([]string{"cfg1", "cfg2", "cfg1"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scope != scopeModeFull {
		t.Fatalf("expected full scope, got %s", scope)
	}
	if len(ids) != 2 || ids[0] != "cfg1" || ids[1] != "cfg2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
