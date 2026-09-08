package request

import (
	"strings"
	"testing"
)

func findingCodesForTest(fs []Finding) []string {
	codes := make([]string, 0, len(fs))
	for _, f := range fs {
		codes = append(codes, f.Check)
	}
	return codes
}

func TestVerifyMessage(t *testing.T) {
	t.Parallel()

	diff := "diff --git a/main.go b/main.go\n+++ b/main.go\n+package main\n"

	cases := []struct {
		name      string
		msg       string
		wrapLine  int
		wantCodes []string
	}{
		{"clean", "fix: resolve leak\n\nFree buffers on error.", 72, nil},
		{"empty", "  \n ", 72, []string{"empty"}},
		{"long subject", strings.Repeat("x", 73), 72, []string{"subject-length"}},
		{"subject exactly at limit", strings.Repeat("x", 72), 72, nil},
		{"long body ok", "fix: ok\n\n" + strings.Repeat("y", 200), 72, nil},
		{"zero wrap uses default", strings.Repeat("x", 73), 0, []string{"subject-length"}},
		{"fence remnant", "fix: ok\n```\ncode\n```", 72, []string{"fence-remnant"}},
		{"backtick remnant", "fix: touch " + "`main.go`" + " now", 72, []string{"fence-remnant"}},
		{"grounded file", "fix: leak in main.go", 72, nil},
		{"ungrounded file", "fix: leak in other.go", 72, []string{"file-grounding"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Join(findingCodesForTest(VerifyMessage(tc.msg, diff, tc.wrapLine)), ",")
			want := strings.Join(tc.wantCodes, ",")

			if got != want {
				t.Errorf("VerifyMessage codes = [%s], want [%s]", got, want)
			}
		})
	}
}
