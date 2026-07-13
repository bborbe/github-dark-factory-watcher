// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeriveTaskID", func() {
	// Pinned namespace — MUST match pkg.darkFactoryWatcherNamespace and MUST
	// differ from the github-pr-watcher namespace so IDs never collide.
	var namespace = uuid.MustParse("b1f4c2a9-7e63-4d81-9a05-3c8e0f5a6d24")

	It("is deterministic — same inputs always produce the same UUID", func() {
		a := pkg.DeriveTaskID("bborbe", "widget", 42, "abc123def456789a")
		b := pkg.DeriveTaskID("bborbe", "widget", 42, "abc123def456789a")
		Expect(a).To(Equal(b))
	})

	It("produces different UUIDs for different owner/repo/number combos", func() {
		a := pkg.DeriveTaskID("bborbe", "widget", 42, "abc123def456789a")
		Expect(a).NotTo(Equal(pkg.DeriveTaskID("bborbe", "widget", 43, "abc123def456789a")))
		Expect(a).NotTo(Equal(pkg.DeriveTaskID("bborbe", "other", 42, "abc123def456789a")))
		Expect(a).NotTo(Equal(pkg.DeriveTaskID("other", "widget", 42, "abc123def456789a")))
	})

	It("produces different UUIDs for the same PR but different SHAs", func() {
		Expect(pkg.DeriveTaskID("bborbe", "widget", 42, "sha-aaa")).
			NotTo(Equal(pkg.DeriveTaskID("bborbe", "widget", 42, "sha-bbb")))
	})

	It("produces the expected pinned UUID from the new namespace", func() {
		expected := uuid.NewSHA1(namespace, []byte("bborbe/widget#42@abc123def456789a"))
		Expect(pkg.DeriveTaskID("bborbe", "widget", 42, "abc123def456789a")).To(Equal(expected))
	})
})
