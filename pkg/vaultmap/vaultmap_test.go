// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vaultmap_test

import (
	"context"

	"github.com/bborbe/github-dark-factory-watcher/pkg/vaultmap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("returns a nil resolver for an empty value so the command keeps its field empty", func() {
		resolver, err := vaultmap.Parse(context.Background(), "")
		Expect(err).To(BeNil())
		Expect(resolver).To(BeNil())
	})

	It("returns a nil resolver for a value of only separators", func() {
		resolver, err := vaultmap.Parse(context.Background(), " , , ")
		Expect(err).To(BeNil())
		Expect(resolver).To(BeNil())
	})

	DescribeTable("rejects a malformed entry",
		func(raw, expected string) {
			resolver, err := vaultmap.Parse(context.Background(), raw)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(expected))
			Expect(resolver).To(BeNil())
		},
		Entry("no separator", "github.com/bborbe/foo", "must be repo-glob=vault"),
		Entry("empty glob", "=personal", "must be repo-glob=vault"),
		Entry("empty vault", "github.com/bborbe/foo=", "must be repo-glob=vault"),
		Entry("vault with uppercase", "github.com/bborbe/foo=Personal", "must match"),
		Entry("vault starting with a digit", "github.com/bborbe/foo=1personal", "must match"),
		Entry("vault with a slash", "github.com/bborbe/foo=personal/x", "must match"),
		Entry("negation", "!github.com/bborbe/foo=personal", "negation is not supported"),
	)
})

var _ = Describe("VaultFor", func() {
	It("maps two repos to two different vaults", func() {
		resolver, err := vaultmap.Parse(
			context.Background(),
			"github.com/bborbe/agent-task-executor=personal,github.com/bborbe/trading=trading",
		)
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/agent-task-executor")).To(Equal("personal"))
		Expect(resolver.VaultFor("github.com/bborbe/trading")).To(Equal("trading"))
	})

	It("falls back to the legacy default for an unmapped repo", func() {
		resolver, err := vaultmap.Parse(context.Background(), "github.com/bborbe/trading=trading")
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/maintainer")).To(Equal(vaultmap.DefaultVault))
		Expect(vaultmap.DefaultVault).To(Equal("openclaw"))
	})

	It("matches an owner wildcard", func() {
		resolver, err := vaultmap.Parse(context.Background(), "github.com/bborbe/*=personal")
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/anything")).To(Equal("personal"))
		Expect(resolver.VaultFor("github.com/other/anything")).To(Equal(vaultmap.DefaultVault))
	})

	It("lets a specific entry win over a later wildcard", func() {
		resolver, err := vaultmap.Parse(
			context.Background(),
			"github.com/bborbe/trading=trading,github.com/bborbe/*=personal",
		)
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/trading")).To(Equal("trading"))
		Expect(resolver.VaultFor("github.com/bborbe/maintainer")).To(Equal("personal"))
	})

	It("lets a wildcard listed first swallow every later specific entry", func() {
		resolver, err := vaultmap.Parse(
			context.Background(),
			"github.com/bborbe/*=personal,github.com/bborbe/trading=trading",
		)
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/trading")).To(Equal("personal"))
	})

	It("ignores surrounding whitespace", func() {
		resolver, err := vaultmap.Parse(
			context.Background(),
			" github.com/bborbe/trading = trading , ",
		)
		Expect(err).To(BeNil())
		Expect(resolver.VaultFor("github.com/bborbe/trading")).To(Equal("trading"))
	})
})
