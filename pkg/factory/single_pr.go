// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory

import (
	"github.com/bborbe/github-dark-factory-watcher/pkg/command"
	"github.com/bborbe/github-dark-factory-watcher/pkg/handler"
)

// CreateSinglePRTriggerHandler wires the thin CQRS handler that publishes a
// TriggerCommand to Kafka for each valid /trigger request. All GitHub and
// candidate-evaluation work lives in the in-pod command consumer (see
// pkg/command.NewTriggerCommandExecutor).
func CreateSinglePRTriggerHandler(
	sender command.TriggerCommandSender,
) handler.SinglePRTriggerHandler {
	return handler.NewSinglePRTriggerHandler(sender)
}
