// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command_test

import (
	"context"
	"encoding/json"

	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/github-dark-factory-watcher/pkg/command"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TriggerCommandOperation", func() {
	It("has expected string value", func() {
		Expect(command.TriggerCommandOperation).
			To(Equal(base.CommandOperation("trigger")))
	})

	It("passes cqrs operation regex validation", func() {
		// Boundary test: catches renames that violate the `^[a-z][a-z-]*$` cqrs
		// wire-string regex (e.g. underscores, leading digit, uppercase).
		Expect(command.TriggerCommandOperation.Validate(context.Background())).
			To(Succeed())
	})
})

var _ = Describe("TriggerCommand", func() {
	It("round-trips through JSON with both fields set", func() {
		cmd := command.TriggerCommand{
			URL:   "https://github.com/bborbe/maintainer/pull/1",
			Force: true,
		}
		data, err := json.Marshal(cmd)
		Expect(err).To(BeNil())

		var got command.TriggerCommand
		Expect(json.Unmarshal(data, &got)).To(Succeed())
		Expect(got.URL).To(Equal(cmd.URL))
		Expect(got.Force).To(Equal(cmd.Force))
	})

	It("omits force when zero (omitempty)", func() {
		cmd := command.TriggerCommand{
			URL: "https://github.com/bborbe/maintainer/pull/1",
		}
		data, err := json.Marshal(cmd)
		Expect(err).To(BeNil())
		Expect(string(data)).NotTo(ContainSubstring("\"force\""))
	})

	It("JSON contains url and force keys when force is set", func() {
		cmd := command.TriggerCommand{
			URL:   "https://github.com/bborbe/maintainer/pull/1",
			Force: true,
		}
		data, err := json.Marshal(cmd)
		Expect(err).To(BeNil())
		jsonStr := string(data)
		Expect(jsonStr).To(ContainSubstring(`"url"`))
		Expect(jsonStr).To(ContainSubstring(`"force"`))
	})

	It("JSON always contains url key", func() {
		cmd := command.TriggerCommand{
			URL: "https://github.com/bborbe/maintainer/pull/1",
		}
		data, err := json.Marshal(cmd)
		Expect(err).To(BeNil())
		Expect(string(data)).To(ContainSubstring(`"url"`))
	})
})

var _ = Describe("TriggerCommand.Validate", func() {
	DescribeTable("Validate",
		func(url string, expectError bool, errSubstring string) {
			cmd := command.TriggerCommand{URL: url}
			err := cmd.Validate(context.Background())
			if expectError {
				Expect(err).To(HaveOccurred())
				if errSubstring != "" {
					Expect(err.Error()).To(ContainSubstring(errSubstring))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		},
		Entry("valid github url",
			"https://github.com/bborbe/maintainer/pull/1", false, ""),
		Entry("empty url",
			"", true, "url must not be empty"),
		Entry("non-url string",
			"not-a-url", true, ""),
		Entry(
			"bitbucket platform",
			"https://bitbucket.example.com/projects/owner/repos/repo/pull-requests/1",
			true,
			"github platform",
		),
		Entry("ftp scheme",
			"ftp://github.com/owner/repo/pull/1", true, ""),
	)
})
