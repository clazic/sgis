package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newParamCmd builds a command with --param plus a couple of defined flags,
// mirroring how real subcommands are wired.
func newParamCmd(args []string) (*cobra.Command, error) {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("year", "", "year")
	registerParamFlag(cmd)
	err := cmd.ParseFlags(args)
	return cmd, err
}

func TestMergeExtraParams_Basic(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "a=1", "--param", "b=2"})
	params := map[string]string{}
	if err := mergeExtraParams(cmd, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["a"] != "1" || params["b"] != "2" {
		t.Errorf("merge result: %v", params)
	}
}

func TestMergeExtraParams_ValueWithEquals(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "a=b=c"})
	params := map[string]string{}
	if err := mergeExtraParams(cmd, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["a"] != "b=c" {
		t.Errorf("expected value 'b=c', got %q", params["a"])
	}
}

func TestMergeExtraParams_NoEquals(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "noequals"})
	if err := mergeExtraParams(cmd, map[string]string{}); err == nil {
		t.Fatal("expected error for missing '='")
	}
}

func TestMergeExtraParams_EmptyKey(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "=value"})
	if err := mergeExtraParams(cmd, map[string]string{}); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestMergeExtraParams_ConflictWithDefinedFlag(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "year=2021"})
	// year already populated by its defined flag
	params := map[string]string{"year": "2020"}
	err := mergeExtraParams(cmd, params)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "year") {
		t.Errorf("conflict error should mention the key: %v", err)
	}
	if params["year"] != "2020" {
		t.Errorf("conflicting --param must not overwrite: %v", params)
	}
}

func TestMergeExtraParams_Trim(t *testing.T) {
	cmd, _ := newParamCmd([]string{"--param", "  k  =  v  "})
	params := map[string]string{}
	if err := mergeExtraParams(cmd, params); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["k"] != "v" {
		t.Errorf("expected trimmed key/value, got %q=%q", "k", params["k"])
	}
}

// TestHelpFlagConsistency guards against the regression fixed in weakness [2]:
// every "--flag" example in a command's Long/help text must correspond to a
// registered flag on that command (or its parent). Scans data/geocode/boundary.
func TestHelpFlagConsistency(t *testing.T) {
	roots := []*cobra.Command{newDataCmd(), newGeocodeCmd(), newBoundaryCmd()}
	for _, root := range roots {
		for _, sub := range root.Commands() {
			checkHelpFlags(t, root, sub)
		}
	}
}

// TestParentHelpFlags checks that every "--flag" in a group's parent Long text
// (e.g. the transcoord example) is registered on at least one of its subcommands.
// This directly guards the weakness [2] fix (--posX/--posY in the geocode group).
func TestParentHelpFlags(t *testing.T) {
	roots := []*cobra.Command{newDataCmd(), newGeocodeCmd(), newBoundaryCmd()}
	for _, root := range roots {
		union := map[string]bool{}
		for _, sub := range root.Commands() {
			sub.Flags().VisitAll(func(f *pflag.Flag) { union[f.Name] = true })
		}
		for _, tok := range strings.Fields(root.Long) {
			if !strings.HasPrefix(tok, "--") {
				continue
			}
			name := strings.TrimRight(strings.TrimPrefix(tok, "--"), ",.`)")
			if name == "" || name == "help" || strings.Contains(name, "=") {
				continue
			}
			if !union[name] && rootCmd.PersistentFlags().Lookup(name) == nil {
				t.Errorf("[%s] parent help references --%s but no subcommand registers it", root.Name(), name)
			}
		}
	}
}

func checkHelpFlags(t *testing.T, parent, sub *cobra.Command) {
	t.Helper()
	for _, tok := range strings.Fields(sub.Long + " " + sub.Example) {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		name := strings.TrimPrefix(tok, "--")
		// strip trailing punctuation
		name = strings.TrimRight(name, ",.`)")
		if name == "" || strings.Contains(name, "=") {
			continue
		}
		if sub.Flags().Lookup(name) == nil &&
			sub.InheritedFlags().Lookup(name) == nil &&
			parent.PersistentFlags().Lookup(name) == nil {
			// root persistent flags (--format/--output) live on the true root.
			if rootCmd.PersistentFlags().Lookup(name) == nil {
				t.Errorf("[%s %s] help references --%s but no such flag is registered", parent.Name(), sub.Name(), name)
			}
		}
	}
}
