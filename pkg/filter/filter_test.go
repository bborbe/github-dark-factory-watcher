// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package filter_test

import (
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskCreationFilters composite", func() {
	It("returns false when slice is empty (vacuous — no filters configured)", func() {
		var fs filter.TaskCreationFilters
		Expect(fs.Skip(filter.PR{})).To(BeFalse())
	})
	It("returns true if any member votes skip", func() {
		fs := filter.TaskCreationFilters{
			filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/allowed"}),
		}
		Expect(fs.Skip(filter.PR{RepoKey: "github.com/bborbe/other"})).To(BeTrue())
	})
	It("returns false when no member votes skip", func() {
		fs := filter.TaskCreationFilters{
			filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/allowed"}),
		}
		Expect(fs.Skip(filter.PR{RepoKey: "github.com/bborbe/allowed"})).To(BeFalse())
	})
	It("supports the function adapter", func() {
		fs := filter.TaskCreationFilters{
			filter.TaskCreationFilterFunc(func(pr filter.PR) bool {
				return pr.RepoKey == "github.com/bborbe/evil"
			}),
		}
		Expect(fs.Skip(filter.PR{RepoKey: "github.com/bborbe/evil"})).To(BeTrue())
		Expect(fs.Skip(filter.PR{RepoKey: "github.com/bborbe/good"})).To(BeFalse())
	})
})
