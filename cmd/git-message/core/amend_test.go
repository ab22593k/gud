package core

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAmendTarget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		flagArgs   []string
		positional []string
		wantRev    string
		wantAmend  bool
	}{
		{"unset", []string{}, nil, "", false},
		{"bare flag means HEAD", []string{"--amend"}, nil, "HEAD", true},
		{"space form", []string{"--amend"}, []string{"HEAD~2"}, "HEAD~2", true},
		{"equals form", []string{"--amend=HEAD~2"}, nil, "HEAD~2", true},
		{"explicit value wins over positional", []string{"--amend=HEAD~1"}, []string{"HEAD~2"}, "HEAD~1", true},
		{"sha rev", []string{"--amend=abc1234"}, nil, "abc1234", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := &cobra.Command{Use: "message"}
			cmd.Flags().String("amend", "", "Amend a previous commit")
			cmd.Flags().Lookup("amend").NoOptDefVal = "HEAD"
			if err := cmd.ParseFlags(tc.flagArgs); err != nil {
				t.Fatalf("ParseFlags(%v)=%v", tc.flagArgs, err)
			}
			gotRev, gotAmend := amendTarget(cmd, tc.positional)
			if gotRev != tc.wantRev || gotAmend != tc.wantAmend {
				t.Errorf("amendTarget(%v,%v)=(%q,%v), want (%q,%v)",
					tc.flagArgs, tc.positional, gotRev, gotAmend, tc.wantRev, tc.wantAmend)
			}
		})
	}
}
