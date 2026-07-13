// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"fmt"
	"strings"

	"github.com/golang/glog"
)

// DefaultMaxTitleLen is the default safety cap for the whole vault filename,
// including segments and separators. Crosses Windows MAX_PATH=260 and ext4
// NAME_MAX=255 with margin. Override via MAX_TITLE_LEN.
const DefaultMaxTitleLen = 200

// DefaultMaxSlugLen is the default cap for the slugified PR-title segment alone.
// Override via MAX_SLUG_LEN.
const DefaultMaxSlugLen = 80

// filenamePrefix names the task kind in the vault filename. It keeps
// dark-factory-implement tasks visually distinct and never collides with a
// pr-review task's filename for the same (PR, SHA).
const filenamePrefix = "Dark Factory Implement"

// computeTaskFilename builds the vault filename (the CreateCommand.Title, which
// the controller turns into "<title>.md"). It MUST be filesystem-safe — no
// slashes, colons, or '#'. Format:
//
//	"Dark Factory Implement <provider> - <owner>-<repo> - <number> - <sha[:8]> - <slug>"
//
// With an empty slug the trailing segment is dropped. taskSuffix, when
// non-empty, is appended as " - <suffix>" (a per-stage disambiguator preserved
// under truncation by shrinking the slug first). The returned string MUST NOT
// include the .md extension; the controller appends it.
func computeTaskFilename(
	provider, owner, repo string,
	number int,
	sha, prTitle string,
	maxSlug, maxTitle int,
	taskSuffix string,
) string {
	shortSHA := sha
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}
	base := fmt.Sprintf(
		"%s %s - %s-%s - %d - %s",
		filenamePrefix,
		provider,
		owner,
		repo,
		number,
		shortSHA,
	)
	slug := slugifyTitle(prTitle, maxSlug)
	t := base
	if slug != "" {
		t = base + " - " + slug
	}
	var suffixPart string
	if taskSuffix != "" {
		suffixPart = " - " + taskSuffix
	}
	if len(t)+len(suffixPart) > maxTitle {
		glog.Warningf(
			"task filename exceeds max length: len=%d max=%d suffix=%q — truncating slug to preserve suffix",
			len(t)+len(suffixPart),
			maxTitle,
			taskSuffix,
		)
		budget := maxTitle - len(suffixPart)
		if budget < 0 {
			budget = 0
		}
		if len(t) > budget {
			t = t[:budget]
		}
	}
	return t + suffixPart
}

// slugifyTitle converts a PR title to a filesystem-safe, human-readable slug.
// Rules (applied in order):
//  1. Lowercase the entire input
//  2. Replace any character that is not [a-z0-9] with a hyphen
//  3. Collapse consecutive hyphens into a single hyphen
//  4. Trim leading and trailing hyphens
//  5. Truncate to maxSlug characters; trim any trailing hyphen left by truncation
//
// Returns empty string if the result after step 4 is empty.
func slugifyTitle(title string, maxSlug int) string {
	lower := strings.ToLower(title)
	var b strings.Builder
	prevHyphen := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > maxSlug {
		result = result[:maxSlug]
		result = strings.TrimRight(result, "-")
	}
	return result
}
