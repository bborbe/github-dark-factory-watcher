// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package filter implements the scope filter chain — the mandatory
// defense-in-depth predicate that refuses to act on any repo outside the
// configured REPO_ALLOWLIST, even if the upstream GitHub search returns more.
//
// This watcher's keep/skip decision for whether a PR carries an
// approved-not-completed dark-factory spec is NOT a pure filter — it needs
// network reads and lives in the pkg/darkfactory package.
package filter

// PR is the filter-evaluation input derived from a GitHub pull request.
// Only the RepoKey is needed for the scope decision.
type PR struct {
	// RepoKey is the host-qualified repo key, e.g. "github.com/bborbe/maintainer".
	RepoKey string
}

//counterfeiter:generate -o ../../mocks/task_creation_filter.go --fake-name TaskCreationFilter . TaskCreationFilter

// TaskCreationFilter decides whether a single PR should be skipped
// (no vault task created). Implementations return true to skip.
type TaskCreationFilter interface {
	// Skip returns true if the PR should be excluded from task creation.
	Skip(pr PR) bool
}

// TaskCreationFilterFunc adapts a function to the TaskCreationFilter
// interface (function-as-implementation, useful for inline filters).
type TaskCreationFilterFunc func(pr PR) bool

// Skip implements TaskCreationFilter for the function adapter.
func (f TaskCreationFilterFunc) Skip(pr PR) bool {
	return f(pr)
}

// TaskCreationFilters is a slice composite: skip if ANY member votes skip.
// An empty slice never skips (no filters configured = process every PR).
type TaskCreationFilters []TaskCreationFilter

// Skip returns true if any contained filter votes skip. Iteration is
// short-circuit on first hit.
func (fs TaskCreationFilters) Skip(pr PR) bool {
	for _, f := range fs {
		if f.Skip(pr) {
			return true
		}
	}
	return false
}
