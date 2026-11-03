// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
)

// DefaultCursorPath is the default path for cursor state persistence.
const DefaultCursorPath = "/data/cursor.json"

// Cursor holds the watcher's persisted poll state, including the last-seen update time and a map of task-identifier to head SHA for force-push detection.
//
// QueuedMarkers is deliberately NOT rebuilt from each poll's search batch, unlike
// HeadSHAs. It records "this PR's queued phase has already been handed to an
// agent", keyed by owner/repo#number rather than by any SHA, and it must outlive
// both the drained window (which spans more than one SHA, since the prompt
// completion and the spec completion are separate commits) and any poll the PR is
// missing from — a PR older than the search watermark, or one skipped on a
// transient error, is absent from the batch and would otherwise lose its entry.
// Entries are dropped when a poll sees the PR's spec reach completed
// (no_spec_in_diff), or — in the narrow race where a PR closes between the search
// and the details fetch — when a poll sees it no longer open (not_open).
//
// A PR merged before either observation is made keeps its entry: the search query
// is `is:pr is:open`, so a closed PR leaves the batch and never reaches processPR
// again. The map therefore grows by roughly one small entry per PR that ever had a
// queued-phase publish, for the lifetime of the deployment — bounded by PR volume,
// not reclaimed. That is accepted over a pruning sweep, because surviving a missing
// batch is precisely what this field exists for; a sweep would reintroduce the
// eviction bug it was added to fix.
type Cursor struct {
	LastUpdatedAt libtime.DateTime  `json:"last_updated_at"`
	HeadSHAs      map[string]string `json:"head_shas"`
	QueuedMarkers map[string]string `json:"queued_markers,omitempty"`
}

// newCursor returns an empty cursor whose maps are ready to write. Both maps must
// be non-nil: processPR writes to them directly, and a nil map panics.
func newCursor(startTime libtime.DateTime) Cursor {
	return Cursor{
		LastUpdatedAt: startTime,
		HeadSHAs:      make(map[string]string),
		QueuedMarkers: make(map[string]string),
	}
}

// LoadCursor reads cursor state from path.
// Returns cold-start state with startTime if the file is missing or corrupt.
func LoadCursor(ctx context.Context, path string, startTime libtime.DateTime) (Cursor, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is config-controlled
	if err != nil {
		if os.IsNotExist(err) {
			glog.V(2).
				Infof("cursor file not found, using cold-start time=%s", startTime.Format(time.RFC3339))
			return newCursor(startTime), nil
		}
		return Cursor{}, errors.Wrap(ctx, err, "read cursor file")
	}
	var state Cursor
	if err := json.Unmarshal(data, &state); err != nil {
		glog.Warningf("cursor file corrupt, using cold-start path=%s err=%v", path, err)
		return newCursor(startTime), nil
	}
	if state.HeadSHAs == nil {
		state.HeadSHAs = make(map[string]string)
	}
	if state.QueuedMarkers == nil {
		state.QueuedMarkers = make(map[string]string)
	}
	return state, nil
}

// SaveCursor persists cursor state to path atomically via a temp file + rename.
func SaveCursor(ctx context.Context, path string, state Cursor) error {
	data, err := json.Marshal(state)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal cursor state")
	}
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil { // #nosec G306 -- intentional 0600
		return errors.Wrap(ctx, err, "write cursor file")
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return errors.Wrap(ctx, err, "rename cursor file")
	}
	return nil
}
