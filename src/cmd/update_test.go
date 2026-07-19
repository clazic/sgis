package cmd

import "testing"

func TestNewerVer(t *testing.T) {
	cases := []struct {
		remote, local string
		want          bool
	}{
		{"v0.1.0", "v0.4.0", false}, // 다운그레이드 방지
		{"v0.4.1", "v0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true}, // 숫자 비교 (문자열 비교면 실패)
		{"v0.4", "v0.4.0", false},
		{"v0.4.1", "dev", true},
		{"v0.4.0", "v0.4.0-dirty", false},
	}
	for _, c := range cases {
		if got := newerVer(c.remote, c.local); got != c.want {
			t.Errorf("newerVer(%q, %q) = %v, want %v", c.remote, c.local, got, c.want)
		}
	}
}
