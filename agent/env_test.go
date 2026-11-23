package agent

import (
	"os"
	"slices"
	"testing"
)

func TestCheckEnvKeepsTheAgentsPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	for _, env := range []map[string]string{{"A": "1"}, {"PATH": "/x" + sep + "$PATH"}, {"PATH": "$PATH" + sep + "/x"}} {
		if err := CheckEnv(env); err != nil {
			t.Errorf("%v: %v", env, err)
		}
	}
	for _, env := range []map[string]string{{"": "1"}, {"A=B": "1"}, {"A": "\x00"}, {"PATH": "/x"}, {"PATH": "$PATH" + sep + "$PATH"}} {
		if CheckEnv(env) == nil {
			t.Errorf("%q passed", env)
		}
	}
}

func TestMergeEnvSetsOverTheBaseAndJoinsPath(t *testing.T) {
	sep := string(os.PathListSeparator)
	base := []string{"A=0", "PATH=/usr/bin"}
	env := MergeEnv(base, map[string]string{"A": "1", "PATH": "/x" + sep + "$PATH"})
	if !slices.Equal(base, []string{"A=0", "PATH=/usr/bin"}) {
		t.Fatalf("base changed: %q", base)
	}
	if EnvValue(env, "A") != "1" || EnvValue(env, "PATH") != "/x"+sep+"/usr/bin" {
		t.Fatalf("merged: %q", env)
	}
	if path := EnvValue(MergeEnv(nil, map[string]string{"PATH": "/x" + sep + "$PATH"}), "PATH"); path != "/x" {
		t.Fatalf("PATH without an inherited one: %q", path)
	}
}
