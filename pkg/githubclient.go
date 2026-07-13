// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"time"

	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	gogithub "github.com/google/go-github/v62/github"
)

// PullRequest holds the fields the watcher needs from a GitHub PR search result.
type PullRequest struct {
	GlobalID  int64
	Number    int
	Owner     string
	Repo      string
	Title     string
	HTMLURL   string
	IsDraft   bool
	UpdatedAt libtime.DateTime
}

// SearchResult is the result of a single paginated search call.
type SearchResult struct {
	PullRequests  []PullRequest
	HasNextPage   bool
	NextPage      int
	RateRemaining int
	RateResetAt   libtime.DateTime
}

// PRDetails holds the per-PR fields the watcher needs to re-confirm state and
// materialize a task the agent can act on. The Search API does not expose most
// of these; they require a follow-up PullRequests.Get call (which also dodges
// search-index lag on the draft/state flags).
type PRDetails struct {
	// HeadSHA is the commit hash of the PR's head branch. Used as the `ref`
	// the agent checks out and as the per-task dedup key.
	HeadSHA string
	// CloneURL is the HTTPS clone URL of the head repo (e.g.
	// `https://github.com/owner/repo.git`). Emitted as `clone_url`.
	CloneURL string
	// Branch is the head branch name (e.g. `dark-factory/foo`). Emitted as `branch`.
	Branch string
	// State is the PR state ("open" / "closed"), re-confirmed via pulls/N.
	State string
	// IsDraft indicates whether the PR is a draft, re-confirmed via pulls/N.
	IsDraft bool
	// Title is the PR title.
	Title string
	// UpdatedAt is the PR last-updated timestamp; drives the cursor watermark.
	UpdatedAt libtime.DateTime
}

//counterfeiter:generate -o ../mocks/github_client.go --fake-name GitHubClient . GitHubClient

// GitHubClient abstracts the GitHub API calls the watcher and the
// darkfactory candidate evaluator need.
type GitHubClient interface {
	// SearchPRs issues a GitHub Search query for open PRs updated since cursor.
	// page=1 for the first call; use SearchResult.NextPage for subsequent calls.
	SearchPRs(
		ctx context.Context,
		scope string,
		since libtime.DateTime,
		page int,
	) (SearchResult, error)

	// GetPRDetails fetches head SHA, clone URL, branch, state and draft flag for
	// a single PR via pulls/N (re-confirming draft/state against search-index lag).
	GetPRDetails(ctx context.Context, owner, repo string, number int) (PRDetails, error)

	// ListPRFiles returns the paths of every file touched by the PR diff
	// (paginated pulls/N/files).
	ListPRFiles(ctx context.Context, owner, repo string, number int) ([]string, error)

	// GetContent returns the decoded bytes of a repo file at the given ref.
	// A missing file surfaces as an error for which IsNotFound reports true.
	GetContent(ctx context.Context, owner, repo, filePath, ref string) ([]byte, error)

	// ListDir returns the entry paths (files and subdirs) directly under the
	// given directory path at ref. A missing directory surfaces as an error for
	// which IsNotFound reports true.
	ListDir(ctx context.Context, owner, repo, dirPath, ref string) ([]string, error)

	// IsNotFound reports whether err is a GitHub 404 (missing file/dir/ref).
	IsNotFound(err error) bool
}

// NewGitHubClient returns a GitHubClient backed by the real GitHub API.
// The httpClient must already carry authentication (App auth via
// lib/githubapp.NewClient).
func NewGitHubClient(httpClient *http.Client) GitHubClient {
	return &githubClient{
		client: gogithub.NewClient(httpClient),
	}
}

type githubClient struct {
	client *gogithub.Client
}

func (c *githubClient) SearchPRs(
	ctx context.Context,
	scope string,
	since libtime.DateTime,
	page int,
) (SearchResult, error) {
	query := fmt.Sprintf(
		"is:pr is:open archived:false user:%s updated:>=%s",
		scope,
		since.Format(time.RFC3339),
	)
	opts := &gogithub.SearchOptions{
		ListOptions: gogithub.ListOptions{
			Page:    page,
			PerPage: 100,
		},
	}

	result, resp, err := c.client.Search.Issues(ctx, query, opts)
	if err != nil {
		return SearchResult{}, errors.Wrapf(ctx, err, "search github prs scope=%s", scope)
	}

	prs := make([]PullRequest, 0, len(result.Issues))
	for _, issue := range result.Issues {
		repoURL := issue.GetRepositoryURL()
		owner, repo := parseOwnerRepo(repoURL)
		prs = append(prs, PullRequest{
			GlobalID:  issue.GetID(),
			Number:    issue.GetNumber(),
			Owner:     owner,
			Repo:      repo,
			Title:     issue.GetTitle(),
			HTMLURL:   issue.GetHTMLURL(),
			IsDraft:   issue.GetDraft(),
			UpdatedAt: libtime.DateTime(issue.GetUpdatedAt().Time),
		})
	}

	return SearchResult{
		PullRequests:  prs,
		HasNextPage:   resp.NextPage > 0,
		NextPage:      resp.NextPage,
		RateRemaining: resp.Rate.Remaining,
		RateResetAt:   libtime.DateTime(resp.Rate.Reset.Time),
	}, nil
}

func (c *githubClient) GetPRDetails(
	ctx context.Context,
	owner, repo string,
	number int,
) (PRDetails, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return PRDetails{}, errors.Wrapf(
			ctx,
			err,
			"get pull request %s/%s#%d",
			owner,
			repo,
			number,
		)
	}
	return PRDetails{
		HeadSHA:   pr.GetHead().GetSHA(),
		CloneURL:  pr.GetHead().GetRepo().GetCloneURL(),
		Branch:    pr.GetHead().GetRef(),
		State:     pr.GetState(),
		IsDraft:   pr.GetDraft(),
		Title:     pr.GetTitle(),
		UpdatedAt: libtime.DateTime(pr.GetUpdatedAt().Time),
	}, nil
}

func (c *githubClient) ListPRFiles(
	ctx context.Context,
	owner, repo string,
	number int,
) ([]string, error) {
	opts := &gogithub.ListOptions{PerPage: 100}
	var files []string
	for {
		commitFiles, resp, err := c.client.PullRequests.ListFiles(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, errors.Wrapf(ctx, err, "list files pr=%s/%s#%d", owner, repo, number)
		}
		for _, f := range commitFiles {
			if name := f.GetFilename(); name != "" {
				files = append(files, name)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return files, nil
}

func (c *githubClient) GetContent(
	ctx context.Context,
	owner, repo, filePath, ref string,
) ([]byte, error) {
	fileContent, _, _, err := c.client.Repositories.GetContents(
		ctx, owner, repo, filePath, &gogithub.RepositoryContentGetOptions{Ref: ref},
	)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "get contents %s/%s:%s@%s", owner, repo, filePath, ref)
	}
	if fileContent == nil {
		return nil, errors.Errorf(ctx, "path %s/%s:%s@%s is not a file", owner, repo, filePath, ref)
	}
	content, err := fileContent.GetContent()
	if err != nil {
		return nil, errors.Wrapf(
			ctx,
			err,
			"decode contents %s/%s:%s@%s",
			owner,
			repo,
			filePath,
			ref,
		)
	}
	return []byte(content), nil
}

func (c *githubClient) ListDir(
	ctx context.Context,
	owner, repo, dirPath, ref string,
) ([]string, error) {
	_, dirContent, _, err := c.client.Repositories.GetContents(
		ctx, owner, repo, dirPath, &gogithub.RepositoryContentGetOptions{Ref: ref},
	)
	if err != nil {
		return nil, errors.Wrapf(ctx, err, "list dir %s/%s:%s@%s", owner, repo, dirPath, ref)
	}
	entries := make([]string, 0, len(dirContent))
	for _, entry := range dirContent {
		if p := entry.GetPath(); p != "" {
			entries = append(entries, p)
		}
	}
	return entries, nil
}

// IsNotFound reports whether err (or any error it wraps) is a GitHub 404.
func (c *githubClient) IsNotFound(err error) bool {
	var errResp *gogithub.ErrorResponse
	if errors.As(err, &errResp) {
		return errResp.Response != nil && errResp.Response.StatusCode == http.StatusNotFound
	}
	return false
}

// parseOwnerRepo extracts owner and repo from a GitHub API repository URL.
// Input format: https://api.github.com/repos/{owner}/{repo}
func parseOwnerRepo(repoURL string) (owner, repo string) {
	dir, repoName := path.Split(repoURL)
	_, ownerName := path.Split(path.Clean(dir))
	return ownerName, repoName
}
