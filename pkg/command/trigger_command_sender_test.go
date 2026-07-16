// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command_test

import (
	"context"

	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	cqrsiam "github.com/bborbe/cqrs/iam"
	cdbmocks "github.com/bborbe/cqrs/mocks"
	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/pkg/command"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	lib "github.com/bborbe/maintainer"
)

// newTestCommandCreator returns a CommandCreator backed by a buffered channel
// pre-populated with `n` request IDs. Suitable for unit tests that don't want to
// plumb base.RequestIDChannel(ctx) and risk the channel blocking.
func newTestCommandCreator(n int) base.CommandCreator {
	ch := make(chan base.RequestID, n)
	for i := 0; i < n; i++ {
		ch <- base.NewRequestID()
	}
	return base.NewCommandCreator(ch)
}

var _ = Describe("TriggerCommandSender.SendCommand", func() {
	var (
		ctx      context.Context
		fakeCDB  *cdbmocks.CDBCommandObjectSender
		sender   command.TriggerCommandSender
		validCmd command.TriggerCommand
	)

	BeforeEach(func() {
		ctx = context.Background()
		fakeCDB = new(cdbmocks.CDBCommandObjectSender)
		sender = command.NewTriggerCommandSender(
			newTestCommandCreator(10),
			cqrsiam.Initiator("test-watcher"),
			fakeCDB,
		)
		validCmd = command.TriggerCommand{
			URL:   "https://github.com/bborbe/repo/pull/42",
			Force: false,
		}
	})

	Context("valid command", func() {
		It("publishes one CommandObject with the correct operation and SchemaID", func() {
			fakeCDB.SendCommandObjectReturns(nil)

			Expect(sender.SendCommand(ctx, validCmd)).To(Succeed())

			Expect(fakeCDB.SendCommandObjectCallCount()).To(Equal(1))
			_, obj := fakeCDB.SendCommandObjectArgsForCall(0)
			Expect(obj.SchemaID).To(Equal(lib.GithubDarkFactoryV1SchemaID))
			Expect(obj.Command.Operation).To(Equal(command.TriggerCommandOperation))
		})
	})

	Context("validation fails", func() {
		It("returns a wrapped validation error and does NOT touch Kafka", func() {
			invalid := command.TriggerCommand{URL: ""}

			err := sender.SendCommand(ctx, invalid)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("validate TriggerCommand"))
			Expect(fakeCDB.SendCommandObjectCallCount()).To(Equal(0))
		})
	})

	Context("Kafka publish fails", func() {
		It("returns a wrapped Kafka error", func() {
			fakeCDB.SendCommandObjectReturns(errors.Errorf(ctx, "broker unavailable"))

			err := sender.SendCommand(ctx, validCmd)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("send TriggerCommand to Kafka"))
			Expect(err.Error()).To(ContainSubstring("broker unavailable"))
			Expect(fakeCDB.SendCommandObjectCallCount()).To(Equal(1))
		})
	})

	Context("downstream is fed the correct command bytes", func() {
		It("populates the event payload from the command via base.ParseEvent", func() {
			// Round-trip via the cdb command's Data: the event the sender
			// constructed must round-trip back to the original TriggerCommand.
			fakeCDB.SendCommandObjectReturns(nil)

			Expect(sender.SendCommand(ctx, validCmd)).To(Succeed())

			_, obj := fakeCDB.SendCommandObjectArgsForCall(0)
			Expect(obj.Command.Data).NotTo(BeNil())
			var roundTripped command.TriggerCommand
			Expect(obj.Command.Data.MarshalInto(ctx, &roundTripped)).To(Succeed())
			Expect(roundTripped.URL).To(Equal(validCmd.URL))
			Expect(roundTripped.Force).To(Equal(validCmd.Force))
		})
	})

	// Silence unused-import warnings if the implementation evolves.
	_ = cdb.CommandObject{}
})
