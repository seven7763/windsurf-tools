package services

import "testing"

func TestIsJWTMintablePoolKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{key: "", want: false},
		{key: "sk-ws-01-abc", want: true},
		{key: "sk-ws-legacy", want: true},
		{key: "devin-session-token$abc", want: true},
		{key: "auth1_xxx", want: false},
		{key: "cog_xxx", want: false},
	}
	for _, tc := range cases {
		if got := isJWTMintablePoolKey(tc.key); got != tc.want {
			t.Fatalf("isJWTMintablePoolKey(%q)=%v want %v", tc.key, got, tc.want)
		}
	}
}
