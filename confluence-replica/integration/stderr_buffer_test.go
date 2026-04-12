package integration

import "testing"

func TestStderrBufferSnapshot(t *testing.T) {
	var buf stderrBuffer

	if _, err := buf.Write([]byte("first line\n")); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if _, err := buf.Write([]byte("second line")); err != nil {
		t.Fatalf("write second chunk: %v", err)
	}

	if got, want := buf.String(), "first line\nsecond line"; got != want {
		t.Fatalf("unexpected snapshot: got %q want %q", got, want)
	}
}
