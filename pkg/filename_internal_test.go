// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("slugifyTitle", func() {
	DescribeTable(
		"produces correct slug",
		func(input string, maxSlug int, want string) {
			Expect(slugifyTitle(input, maxSlug)).To(Equal(want))
		},
		Entry("simple lowercase", "fix bug", DefaultMaxSlugLen, "fix-bug"),
		Entry("uppercase converted", "Fix Bug", DefaultMaxSlugLen, "fix-bug"),
		Entry(
			"special chars replaced",
			"feat: new-feature!",
			DefaultMaxSlugLen,
			"feat-new-feature",
		),
		Entry("consecutive special collapsed", "hello   world", DefaultMaxSlugLen, "hello-world"),
		Entry("leading special stripped", "!leading", DefaultMaxSlugLen, "leading"),
		Entry("trailing special stripped", "trailing!", DefaultMaxSlugLen, "trailing"),
		Entry("only special → empty", "!!!", DefaultMaxSlugLen, ""),
		Entry("empty → empty", "", DefaultMaxSlugLen, ""),
		Entry("unicode-only → empty", "🚀🎉", DefaultMaxSlugLen, ""),
		Entry("mixed unicode and ascii", "fix 🐛 bug", DefaultMaxSlugLen, "fix-bug"),
		Entry("digits preserved", "v1 release", DefaultMaxSlugLen, "v1-release"),
		Entry("already slug-safe", "my-feature", DefaultMaxSlugLen, "my-feature"),
		Entry(
			"truncation at custom cap 50 trims trailing hyphen",
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm-extra-words-here",
			50,
			"abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklm",
		),
		Entry(
			"pr title with colon",
			"feat: add new endpoint",
			DefaultMaxSlugLen,
			"feat-add-new-endpoint",
		),
		Entry("pr title with slash", "fix/auth bug", DefaultMaxSlugLen, "fix-auth-bug"),
		Entry("pr title with dots", "bump v1.2.3", DefaultMaxSlugLen, "bump-v1-2-3"),
	)
})

var _ = Describe("computeTaskFilename", func() {
	DescribeTable(
		"produces correct filename",
		func(provider, owner, repo string, number int, sha, title string, maxSlug, maxTitle int, taskSuffix, want string) {
			Expect(
				computeTaskFilename(
					provider,
					owner,
					repo,
					number,
					sha,
					title,
					maxSlug,
					maxTitle,
					taskSuffix,
				),
			).To(Equal(want))
		},
		Entry("normal PR with title",
			"github", "bborbe", "widget", 2, "abc12345def67890", "feat: add thing",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "",
			"Dark Factory Implement github - bborbe-widget - 2 - abc12345 - feat-add-thing"),
		Entry("empty title → no slug segment",
			"github", "bborbe", "x", 7, "abc12345def67890", "",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "",
			"Dark Factory Implement github - bborbe-x - 7 - abc12345"),
		Entry("unicode-only title → no slug segment",
			"github", "bborbe", "x", 7, "abc12345def67890", "🚀🎉",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "",
			"Dark Factory Implement github - bborbe-x - 7 - abc12345"),
		Entry("short SHA — no truncation",
			"github", "bborbe", "repo", 1, "abc", "my title",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "",
			"Dark Factory Implement github - bborbe-repo - 1 - abc - my-title"),
		Entry("hyphenated names joined",
			"github", "my-org", "my-repo", 99, "abc12345def67890", "bump deps",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "",
			"Dark Factory Implement github - my-org-my-repo - 99 - abc12345 - bump-deps"),
		Entry("suffix=dev appended",
			"github", "bborbe", "repo", 12, "76fe3e86def67890", "improve readme",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "dev",
			"Dark Factory Implement github - bborbe-repo - 12 - 76fe3e86 - improve-readme - dev"),
		Entry("suffix=dev with unicode-only title — suffix follows sha directly",
			"github", "bborbe", "repo", 1, "abc12345def67890", "🚀🎉",
			DefaultMaxSlugLen, DefaultMaxTitleLen, "dev",
			"Dark Factory Implement github - bborbe-repo - 1 - abc12345 - dev"),
	)
})
