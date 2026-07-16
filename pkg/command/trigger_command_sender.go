// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command

import (
	"context"

	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	cqrsiam "github.com/bborbe/cqrs/iam"
	"github.com/bborbe/errors"
	"github.com/golang/glog"

	lib "github.com/bborbe/maintainer"
)

//counterfeiter:generate -o ../../mocks/trigger_command_sender.go --fake-name TriggerCommandSender . TriggerCommandSender

// TriggerCommandSender sends TriggerCommand payloads to Kafka. Calls Validate
// before publishing — a validation error is returned without touching Kafka.
type TriggerCommandSender interface {
	SendCommand(ctx context.Context, cmd TriggerCommand) error
}

// NewTriggerCommandSender creates a TriggerCommandSender. The commandCreator and
// initiator are injected at construction time per the
// cqrs/docs/producing-commands.md "Factory Wiring" pattern — built once at
// wiring, reused across every SendCommand call. The commandObjectSender wraps
// the Kafka sync producer.
func NewTriggerCommandSender(
	commandCreator base.CommandCreator,
	initiator cqrsiam.Initiator,
	commandObjectSender cdb.CommandObjectSender,
) TriggerCommandSender {
	return &triggerCommandSender{
		commandCreator:      commandCreator,
		initiator:           initiator,
		commandObjectSender: commandObjectSender,
	}
}

type triggerCommandSender struct {
	commandCreator      base.CommandCreator
	initiator           cqrsiam.Initiator
	commandObjectSender cdb.CommandObjectSender
}

func (s *triggerCommandSender) SendCommand(
	ctx context.Context,
	cmd TriggerCommand,
) error {
	if err := cmd.Validate(ctx); err != nil {
		return errors.Wrapf(ctx, err, "validate TriggerCommand")
	}
	event, err := base.ParseEvent(ctx, cmd)
	if err != nil {
		return errors.Wrapf(ctx, err, "parse TriggerCommand event")
	}
	commandObject := cdb.CommandObject{
		Command: s.commandCreator.NewCommand(
			TriggerCommandOperation,
			s.initiator,
			"",
			event,
		),
		SchemaID: lib.GithubDarkFactoryV1SchemaID,
	}
	if err := s.commandObjectSender.SendCommandObject(ctx, commandObject); err != nil {
		return errors.Wrapf(ctx, err, "send TriggerCommand to Kafka")
	}
	glog.V(2).
		Infof("trigger sender: published op=%s url=%s", TriggerCommandOperation, cmd.URL)
	return nil
}
