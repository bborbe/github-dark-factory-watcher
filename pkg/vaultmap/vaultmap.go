// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vaultmap

import (
	"context"
	"regexp"
	"strings"

	"github.com/bborbe/errors"
	"github.com/bborbe/validation"

	"github.com/bborbe/maintainer/repoallowlist"
)

// DefaultVault is the vault an unmapped repo falls back to. It mirrors the
// task controller's legacy default (agent-task-controller pkg/routing
// LegacyDefaultVault), so a map that omits a repo routes exactly as it did
// before the mapping existed.
const DefaultVault = "openclaw"

// vaultSlugRegexp mirrors the controller's VAULT_NAME validation. A slug that
// fails it can never be claimed by any controller, so it is rejected at parse
// time instead of producing commands that are silently dropped at the far end.
var vaultSlugRegexp = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

//counterfeiter:generate -o ../../mocks/resolver.go --fake-name Resolver . Resolver

// Resolver returns the vault slug for a host-qualified repo key such as
// "github.com/bborbe/trading".
type Resolver interface {
	VaultFor(repoKey string) string
}

// Parse parses a REPO_VAULT_MAP value into a Resolver.
//
// The value is a comma-separated list of repo-glob=vault entries. repo-glob
// accepts the same wildcard syntax as REPO_ALLOWLIST (e.g.
// "github.com/bborbe/*"). Entries are evaluated in order and the first match
// wins, so a specific repo must be listed before a wildcard that also covers
// it. Negation entries ("!repo") are rejected: a vault map has no sensible
// meaning for "not this repo", and accepting one would make it match
// everything.
//
// An empty value returns (nil, nil): the command keeps its field empty and the
// controller's legacy default applies, which is byte-identical to the
// behaviour before this mapping existed.
func Parse(ctx context.Context, raw string) (Resolver, error) {
	var entries []entry
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		glob, vault, found := strings.Cut(part, "=")
		glob = strings.TrimSpace(glob)
		vault = strings.TrimSpace(vault)
		if !found || glob == "" || vault == "" {
			return nil, errors.Wrapf(
				ctx, validation.Error,
				"repo-vault-map entry %q must be repo-glob=vault", part,
			)
		}
		if strings.HasPrefix(glob, "!") {
			return nil, errors.Wrapf(
				ctx, validation.Error,
				"repo-vault-map entry %q: negation is not supported", part,
			)
		}
		if !vaultSlugRegexp.MatchString(vault) {
			return nil, errors.Wrapf(
				ctx, validation.Error,
				"repo-vault-map entry %q: vault %q must match %s", part, vault, vaultSlugRegexp,
			)
		}
		entries = append(entries, entry{glob: glob, vault: vault})
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &resolver{entries: entries}, nil
}

type entry struct {
	glob  string
	vault string
}

type resolver struct {
	entries []entry
}

// VaultFor returns the vault of the first entry whose glob matches repoKey,
// or DefaultVault when no entry matches.
func (r *resolver) VaultFor(repoKey string) string {
	for _, e := range r.entries {
		if repoallowlist.IsAllowed([]string{e.glob}, repoKey) {
			return e.vault
		}
	}
	return DefaultVault
}
