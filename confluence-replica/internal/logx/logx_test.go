package logx

import "testing"

func TestDetermineLevelFromConfig(t *testing.T) {
	lvl, err := DetermineLevel("INFO", false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lvl != LevelInfo {
		t.Fatalf("unexpected level: %v", lvl)
	}
}

func TestDetermineLevelQuiet(t *testing.T) {
	lvl, err := DetermineLevel("DEBUG", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lvl != LevelError {
		t.Fatalf("quiet must force ERROR")
	}
}

func TestDetermineLevelVerbose(t *testing.T) {
	lvl, err := DetermineLevel("ERROR", false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lvl != LevelDebug {
		t.Fatalf("verbose must force DEBUG")
	}
}

func TestDetermineLevelMutuallyExclusive(t *testing.T) {
	_, err := DetermineLevel("INFO", true, true)
	if err == nil {
		t.Fatalf("expected error")
	}
}
