// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"

	"github.com/google/uuid"
)

// darkFactoryWatcherNamespace is the fixed v5 UUID namespace for all
// task identifiers derived by this watcher. It is a distinct random value
// (NOT the github-pr-watcher namespace) so a dark-factory-implement task can
// never collide with a pr-review task for the same (owner, repo, number, sha).
// Changing it invalidates all existing task identifiers.
var darkFactoryWatcherNamespace = uuid.MustParse("b1f4c2a9-7e63-4d81-9a05-3c8e0f5a6d24")

// DeriveTaskID returns a deterministic task identifier for a (PR, SHA) pair.
// Input: "<owner>/<repo>#<number>@<sha>", e.g. "bborbe/maintainer#42@abc123...".
// The full SHA is used (not truncated) to keep the dedup keyspace collision-free.
// Same inputs → same UUID, so a re-emit of the same (PR, SHA) is a controller no-op.
func DeriveTaskID(owner, repo string, number int, sha string) uuid.UUID {
	key := fmt.Sprintf("%s/%s#%d@%s", owner, repo, number, sha)
	return uuid.NewSHA1(darkFactoryWatcherNamespace, []byte(key))
}
