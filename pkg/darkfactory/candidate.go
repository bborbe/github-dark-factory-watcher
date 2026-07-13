// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package darkfactory

import (
	"context"
	"strings"

	"github.com/bborbe/errors"
	"github.com/golang/glog"
	yaml "gopkg.in/yaml.v3"
)

// darkFactoryConfigFile is the per-repo config that must be present on the
// draft-PR branch for dark-factory to run against it.
const darkFactoryConfigFile = ".dark-factory.yaml"

// specDir is the directory (relative to repo root) holding in-progress specs.
// Specs persist here across their whole lifecycle, so an existence-only check
// false-triggers every unrelated draft PR — the match MUST be scoped to the PR
// diff (see Evaluate).
const specDir = "specs/in-progress"

// promptsDir holds the agent's own in-flight prompts. Any *.md here at head is
// a self-trigger signal: the agent's commits keep the spec approved-not-completed
// while it works, so the watcher must not re-emit.
const promptsDir = "prompts/in-progress"

// Skip reason labels (also the IncFilterSkipped metric labels).
const (
	ReasonNotDraft         = "not_draft"
	ReasonNotOpen          = "not_open"
	ReasonNoDarkFactoryYML = "no_dark_factory_yaml"
	ReasonNoSpecInDiff     = "no_spec_in_diff"
	ReasonPromptsInFlight  = "prompts_in_flight"
)

//counterfeiter:generate -o ../../mocks/content_reader.go --fake-name ContentReader . ContentReader

// ContentReader is the narrow GitHub read surface the evaluator needs. It is
// declared here (with primitive returns only) so this package does not import
// pkg and the concrete *githubClient satisfies it structurally without an
// adapter or an import cycle.
type ContentReader interface {
	ListPRFiles(ctx context.Context, owner, repo string, number int) ([]string, error)
	GetContent(ctx context.Context, owner, repo, filePath, ref string) ([]byte, error)
	ListDir(ctx context.Context, owner, repo, dirPath, ref string) ([]string, error)
	IsNotFound(err error) bool
}

// Input is the per-PR data the evaluator decides on. IsDraft and State are the
// pulls/N-confirmed values (dodging search-index lag), not the search snapshot.
type Input struct {
	Owner   string
	Repo    string
	Number  int
	HeadSHA string
	IsDraft bool
	State   string
}

// Result is the keep/skip verdict. Reason is the metric label for a skip and is
// empty when Keep is true. MatchedSpecs lists the approved-not-completed spec
// paths that satisfied the diff check (populated only when Keep is true).
type Result struct {
	Keep         bool
	Reason       string
	MatchedSpecs []string
}

func skip(reason string) Result { return Result{Keep: false, Reason: reason} }

// Evaluate returns Keep=true only when ALL hold at in.HeadSHA:
//
//	a. the PR is open and a draft;
//	b. .dark-factory.yaml is present;
//	c. at least one approved-not-completed spec under specs/in-progress/ is
//	   touched by the PR diff (existence alone is NOT enough — specs persist
//	   across their lifecycle);
//	d. no *.md remains under prompts/in-progress/ (self-trigger suppression).
//
// A non-404 GitHub error is returned to the caller (cursor not advanced); a 404
// on an expected-optional path is treated as "absent" and drives a skip verdict.
func Evaluate(ctx context.Context, reader ContentReader, in Input) (Result, error) {
	if !in.IsDraft {
		return skip(ReasonNotDraft), nil
	}
	if in.State != "open" {
		return skip(ReasonNotOpen), nil
	}

	if _, err := reader.GetContent(ctx, in.Owner, in.Repo, darkFactoryConfigFile, in.HeadSHA); err != nil {
		if reader.IsNotFound(err) {
			return skip(ReasonNoDarkFactoryYML), nil
		}
		return Result{}, errors.Wrapf(ctx, err, "get %s", darkFactoryConfigFile)
	}

	inFlight, err := hasInFlightPrompts(ctx, reader, in)
	if err != nil {
		return Result{}, errors.Wrap(ctx, err, "check in-flight prompts")
	}
	if inFlight {
		return skip(ReasonPromptsInFlight), nil
	}

	matched, err := matchedApprovedSpecsInDiff(ctx, reader, in)
	if err != nil {
		return Result{}, errors.Wrap(ctx, err, "check approved spec in diff")
	}
	if len(matched) == 0 {
		return skip(ReasonNoSpecInDiff), nil
	}
	return Result{Keep: true, MatchedSpecs: matched}, nil
}

// hasInFlightPrompts reports whether any *.md remains under prompts/in-progress/
// at head. A missing directory (404) counts as none.
func hasInFlightPrompts(ctx context.Context, reader ContentReader, in Input) (bool, error) {
	entries, err := reader.ListDir(ctx, in.Owner, in.Repo, promptsDir, in.HeadSHA)
	if err != nil {
		if reader.IsNotFound(err) {
			return false, nil
		}
		return false, errors.Wrapf(ctx, err, "list %s", promptsDir)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry, ".md") {
			return true, nil
		}
	}
	return false, nil
}

// matchedApprovedSpecsInDiff returns the paths of every spec under
// specs/in-progress/ that the PR diff touches and that is approved-but-not-
// completed at head.
func matchedApprovedSpecsInDiff(
	ctx context.Context,
	reader ContentReader,
	in Input,
) ([]string, error) {
	files, err := reader.ListPRFiles(ctx, in.Owner, in.Repo, in.Number)
	if err != nil {
		return nil, errors.Wrapf(
			ctx,
			err,
			"list pr files pr=%s/%s#%d",
			in.Owner,
			in.Repo,
			in.Number,
		)
	}
	prefix := specDir + "/"
	var matched []string
	for _, file := range files {
		if !strings.HasPrefix(file, prefix) || !strings.HasSuffix(file, ".md") {
			continue
		}
		content, err := reader.GetContent(ctx, in.Owner, in.Repo, file, in.HeadSHA)
		if err != nil {
			if reader.IsNotFound(err) {
				// Touched by the diff (e.g. moved/deleted) but absent at head — ignore.
				glog.V(3).Infof("spec %s absent at head, skipping", file)
				continue
			}
			return nil, errors.Wrapf(ctx, err, "get spec %s", file)
		}
		approved, err := specApprovedNotCompleted(ctx, content)
		if err != nil {
			return nil, errors.Wrapf(ctx, err, "parse spec %s", file)
		}
		if approved {
			glog.V(2).Infof("approved-not-completed spec in diff pr=%s/%s#%d spec=%s",
				in.Owner, in.Repo, in.Number, file)
			matched = append(matched, file)
		}
	}
	return matched, nil
}

// specApprovedNotCompleted reports whether a spec's YAML frontmatter has
// `approved:` set to a non-empty value AND `completed:` absent or empty.
func specApprovedNotCompleted(ctx context.Context, content []byte) (bool, error) {
	fm, err := parseFrontmatter(ctx, content)
	if err != nil {
		return false, errors.Wrap(ctx, err, "parse frontmatter")
	}
	return frontmatterSet(fm, "approved") && !frontmatterSet(fm, "completed"), nil
}

// frontmatterSet reports whether key is present with a non-empty value.
func frontmatterSet(fm map[string]interface{}, key string) bool {
	v, ok := fm[key]
	if !ok || v == nil {
		return false
	}
	if s, isStr := v.(string); isStr {
		return strings.TrimSpace(s) != ""
	}
	return true
}

// parseFrontmatter extracts the leading `---`-delimited YAML block. Content
// without a leading frontmatter block yields an empty map (no error).
func parseFrontmatter(ctx context.Context, content []byte) (map[string]interface{}, error) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") && text != "---" {
		return map[string]interface{}{}, nil
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return map[string]interface{}{}, nil
	}
	block := rest[:end]
	fm := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(block), &fm); err != nil {
		return nil, errors.Wrap(ctx, err, "unmarshal frontmatter yaml")
	}
	if fm == nil {
		return map[string]interface{}{}, nil
	}
	return fm, nil
}
