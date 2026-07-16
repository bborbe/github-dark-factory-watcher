// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	"github.com/bborbe/validation"

	"github.com/bborbe/maintainer/prurl"
)

// TriggerCommandOperation is the Kafka command operation for force-triggering a
// single draft PR into the dark-factory-implement pipeline. Wire string: "trigger".
const TriggerCommandOperation base.CommandOperation = "trigger"

// TriggerCommand is the payload for TriggerCommandOperation. It is published to
// the github-dark-factory watcher's request topic by the /trigger HTTP handler
// and consumed by the in-pod command consumer.
//
// URL is the GitHub draft-PR URL the operator wants pushed into the pipeline.
// Force, when true, causes the executor to derive a salted TaskIdentifier via
// pkg.DeriveTaskIDForce (microsecond-resolution time nonce) so the agent
// controller's vault-file dedup skip does NOT fire and a fresh implement task
// is created for the same head SHA.
type TriggerCommand struct {
	URL   string `json:"url"`
	Force bool   `json:"force,omitempty"`
}

// Validate enforces the command's schema rules. URL must be non-empty and must
// parse to a GitHub-platform PR URL (same rules as the HTTP handler's
// validateTriggerURL — see pkg/handler/trigger_handler.go). Non-GitHub
// platforms and unparseable URLs are rejected here so a buggy client that
// bypasses the HTTP layer cannot enqueue garbage.
func (cmd TriggerCommand) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("URL", validation.HasValidationFunc(func(ctx context.Context) error {
			if cmd.URL == "" {
				return errors.Wrap(ctx, validation.Error, "url must not be empty")
			}
			prInfo, err := prurl.ParsePRURL(ctx, cmd.URL)
			if err != nil {
				return errors.Wrapf(ctx, err, "parse url %q", cmd.URL)
			}
			if prInfo.Platform != prurl.PlatformGitHub {
				return errors.Errorf(
					ctx,
					"only github platform is supported, got %s",
					prInfo.Platform,
				)
			}
			return nil
		})),
	}.Validate(ctx)
}
