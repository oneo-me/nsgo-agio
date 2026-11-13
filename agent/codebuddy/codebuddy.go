// Package codebuddy runs Tencent's CodeBuddy Code CLI through its ACP
// server, codebuddy --acp.
package codebuddy

import (
	"errors"

	"github.com/oneo-me/nsgo-agio/agent"
	"github.com/oneo-me/nsgo-agio/agent/acp"
)

// ErrNotFound reports that codebuddy is not on the login PATH.
var ErrNotFound = errors.New("codebuddy was not found on PATH")

// Spec describes CodeBuddy Code's ACP server. Full Access skips every
// permission check; older versions offer bypassPermissions instead.
var Spec = acp.Spec{
	Tool:     agent.CodeBuddy,
	Provider: "CodeBuddy",
	Args:     []string{"--acp"},
	Modes:    []string{"fullAccess", "bypassPermissions"},
}

// Locate finds codebuddy, or its short name cbc, as agent.LocateScript
// does: a Bun global install runs under Bun.
func Locate(env []string) (*acp.Driver, error) {
	path, args := agent.LocateScript("codebuddy", env)
	if path == "" {
		path, args = agent.LocateScript("cbc", env)
	}
	if path == "" {
		return nil, ErrNotFound
	}
	return &acp.Driver{Spec: Spec, Path: path, PathArgs: args, Env: env}, nil
}
