package cmd

import "testing"

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name    string
		drifted int
		errs    int
		want    int
	}{
		{"clean", 0, 0, 0},
		{"drift only", 2, 0, 2},
		{"error only", 0, 1, 1},
		{"errors win over drift", 3, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exitCodeFor(c.drifted, c.errs); got != c.want {
				t.Errorf("exitCodeFor(%d, %d) = %d, want %d", c.drifted, c.errs, got, c.want)
			}
		})
	}
}
