package version

import "testing"

func TestMCPVersion(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	cases := []struct {
		in   string
		want string
	}{
		{in: "dev", want: "dev"},
		{in: "", want: "dev"},
		{in: "0.3.1", want: "0.3.1"},
		{in: "v0.3.1", want: "0.3.1"},
		{in: "confluence-replica/v0.3.1", want: "0.3.1"},
	}

	for _, tc := range cases {
		Version = tc.in
		got := MCPVersion()
		if got != tc.want {
			t.Fatalf("MCPVersion(%q): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
