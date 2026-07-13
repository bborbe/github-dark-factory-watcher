// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg

import (
	"context"
	"fmt"

	agentlib "github.com/bborbe/agent"
	task "github.com/bborbe/agent/command/task"
	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/pkg/darkfactory"
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
)

//counterfeiter:generate -o ../mocks/watcher.go --fake-name Watcher . Watcher

// Watcher polls GitHub and publishes dark-factory-implement task commands to Kafka.
type Watcher interface {
	Poll(ctx context.Context) error
}

// TaskConfig groups the per-task publishing configuration.
type TaskConfig struct {
	Stage       string
	MaxSlugLen  int
	MaxTitleLen int
	TaskSuffix  string
	// TargetVault routes the CreateTaskCommand to a specific vault controller
	// (matched verbatim against the controller's VAULT_NAME). Empty leaves it
	// unset so the controller's legacy default-vault fallback applies.
	TargetVault string
}

// NewWatcher returns a Watcher that polls GitHub and publishes commands.
func NewWatcher(
	ghClient GitHubClient,
	createSender task.CreateCommandSender,
	metrics Metrics,
	cursorPath string,
	startTime libtime.DateTime,
	scope string,
	scopeFilter filter.TaskCreationFilter,
	cfg TaskConfig,
) Watcher {
	return &watcher{
		ghClient:     ghClient,
		createSender: createSender,
		metrics:      metrics,
		cursorPath:   cursorPath,
		startTime:    startTime,
		scope:        scope,
		scopeFilter:  scopeFilter,
		cfg:          cfg,
	}
}

type watcher struct {
	ghClient     GitHubClient
	createSender task.CreateCommandSender
	metrics      Metrics
	cursorPath   string
	startTime    libtime.DateTime
	scope        string
	scopeFilter  filter.TaskCreationFilter
	cfg          TaskConfig
}

func (w *watcher) Poll(ctx context.Context) error {
	cursorState, err := LoadCursor(ctx, w.cursorPath, w.startTime)
	if err != nil {
		return errors.Wrapf(ctx, err, "load cursor")
	}

	allPRs, abortReason := w.fetchAllPRs(ctx, cursorState.LastUpdatedAt)
	if abortReason != "" {
		w.metrics.IncPollCycle(abortReason)
		return nil
	}

	select {
	case <-ctx.Done():
		return nil
	default:
	}

	maxUpdatedAt := w.processPRs(ctx, &cursorState, allPRs)
	if maxUpdatedAt.After(cursorState.LastUpdatedAt) {
		cursorState.LastUpdatedAt = maxUpdatedAt
	}

	if err := SaveCursor(ctx, w.cursorPath, cursorState); err != nil {
		glog.Errorf("failed to save cursor err=%v", err)
	}
	w.metrics.IncPollCycle("success")
	return nil
}

// fetchAllPRs paginates GitHub search results. Returns (prs, "") on success,
// or (nil, reason) where reason is "github_error" if the caller should abort.
func (w *watcher) fetchAllPRs(
	ctx context.Context,
	since libtime.DateTime,
) ([]PullRequest, string) {
	page := 1
	var allPRs []PullRequest
	for {
		select {
		case <-ctx.Done():
			glog.V(2).Infof("fetchAllPRs cancelled before page search")
			return nil, ""
		default:
		}

		result, err := w.ghClient.SearchPRs(ctx, w.scope, since, page)
		if err != nil {
			glog.Errorf("github search failed err=%v", err)
			return nil, "github_error"
		}
		allPRs = append(allPRs, result.PullRequests...)
		if !result.HasNextPage {
			break
		}
		page = result.NextPage
	}
	return allPRs, ""
}

// processPRs iterates over fetched PRs, publishes commands, and returns the max
// updated-at seen. It rebuilds HeadSHAs from only the current open-PR batch,
// pruning closed/merged PRs.
//
// CRITICAL ASSUMPTION: the controller deduplicates incoming CreateTaskCommands
// by their task_identifier (UUID5). Filtered or transiently-failed PRs are NOT
// added to the new cursor map, so on the next successful poll they are
// re-evaluated and (if still a candidate) re-published — controller dedup makes
// that re-emit a no-op. If the controller ever stops deduping, this design must
// change to preserve per-PR cursor entries.
func (w *watcher) processPRs(
	ctx context.Context,
	cursorState *Cursor,
	allPRs []PullRequest,
) libtime.DateTime {
	maxUpdatedAt := cursorState.LastUpdatedAt
	newHeadSHAs := make(map[string]string, len(allPRs))

	for _, pr := range allPRs {
		select {
		case <-ctx.Done():
			glog.V(2).Infof("poll cancelled during processPRs at pr %d", pr.Number)
			cursorState.HeadSHAs = newHeadSHAs
			return maxUpdatedAt
		default:
		}
		if updatedAt, ok := w.processPR(ctx, pr, cursorState, newHeadSHAs); ok {
			if updatedAt.After(maxUpdatedAt) {
				maxUpdatedAt = updatedAt
			}
		}
	}
	cursorState.HeadSHAs = newHeadSHAs
	return maxUpdatedAt
}

// processPR processes a single PR: scope filter → details → candidate eval →
// dedup → publish. It returns (pr.UpdatedAt, true) when the PR advanced cursor
// state (deduped or published) and (zero, false) when it was skipped (filtered,
// error, non-candidate, or publish failure) and must not advance the cursor.
func (w *watcher) processPR(
	ctx context.Context,
	pr PullRequest,
	cursorState *Cursor,
	newHeadSHAs map[string]string,
) (libtime.DateTime, bool) {
	w.metrics.IncReposScanned()

	repoKey := "github.com/" + pr.Owner + "/" + pr.Repo
	if w.scopeFilter.Skip(filter.PR{RepoKey: repoKey}) {
		glog.V(3).Infof("skipping pr=%s/%s#%d reason=out_of_scope", pr.Owner, pr.Repo, pr.Number)
		w.metrics.IncFilterSkipped("out_of_scope")
		return libtime.DateTime{}, false
	}

	details, err := w.ghClient.GetPRDetails(ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		glog.Errorf("get pr details failed pr=%s/%s#%d err=%v", pr.Owner, pr.Repo, pr.Number, err)
		w.metrics.IncFilterSkipped("details_error")
		return libtime.DateTime{}, false
	}
	if details.HeadSHA == "" {
		glog.Warningf("missing head SHA for pr=%s/%s#%d, skipping", pr.Owner, pr.Repo, pr.Number)
		w.metrics.IncFilterSkipped("empty_sha")
		return libtime.DateTime{}, false
	}

	result, err := darkfactory.Evaluate(ctx, w.ghClient, darkfactory.Input{
		Owner:   pr.Owner,
		Repo:    pr.Repo,
		Number:  pr.Number,
		HeadSHA: details.HeadSHA,
		IsDraft: details.IsDraft,
		State:   details.State,
	})
	if err != nil {
		glog.Errorf(
			"evaluate candidate failed pr=%s/%s#%d err=%v",
			pr.Owner,
			pr.Repo,
			pr.Number,
			err,
		)
		w.metrics.IncFilterSkipped("evaluate_error")
		return libtime.DateTime{}, false
	}
	if !result.Keep {
		glog.V(3).
			Infof("skipping pr=%s/%s#%d reason=%s", pr.Owner, pr.Repo, pr.Number, result.Reason)
		w.metrics.IncFilterSkipped(result.Reason)
		return libtime.DateTime{}, false
	}

	taskIDStr := DeriveTaskID(pr.Owner, pr.Repo, pr.Number, details.HeadSHA).String()
	if _, exists := cursorState.HeadSHAs[taskIDStr]; exists {
		glog.V(3).Infof("no change, skipping pr=%s/%s#%d sha=%s taskID=%s",
			pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr)
		newHeadSHAs[taskIDStr] = details.HeadSHA
		return pr.UpdatedAt, true
	}

	if w.publish(ctx, pr, details, taskIDStr, result.MatchedSpecs) {
		cursorState.HeadSHAs[taskIDStr] = details.HeadSHA
		newHeadSHAs[taskIDStr] = details.HeadSHA
		return pr.UpdatedAt, true
	}
	return libtime.DateTime{}, false
}

// publish builds and sends the CreateTaskCommand. Returns true on success.
func (w *watcher) publish(
	ctx context.Context,
	pr PullRequest,
	details PRDetails,
	taskIDStr string,
	matchedSpecs []string,
) bool {
	cmd := BuildCreateCommand(pr, details, taskIDStr, w.cfg, matchedSpecs)
	if err := w.createSender.SendCommand(ctx, cmd); err != nil {
		glog.Errorf(
			"publish create-task failed pr=%s/%s#%d err=%v",
			pr.Owner,
			pr.Repo,
			pr.Number,
			err,
		)
		w.metrics.IncPublished("error")
		return false
	}
	glog.V(2).Infof("published CreateTaskCommand pr=%s/%s#%d sha=%s taskID=%s",
		pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr)
	w.metrics.IncPublished("create")
	return true
}

// BuildCreateCommand builds the dark-factory-implement CreateTaskCommand for a
// candidate PR. Exposed for direct testing of the emitted contract.
func BuildCreateCommand(
	pr PullRequest,
	details PRDetails,
	taskIDStr string,
	cfg TaskConfig,
	matchedSpecs []string,
) task.CreateCommand {
	return task.CreateCommand{
		Title: computeTaskFilename(
			"github",
			pr.Owner,
			pr.Repo,
			pr.Number,
			details.HeadSHA,
			details.Title,
			cfg.MaxSlugLen,
			cfg.MaxTitleLen,
			cfg.TaskSuffix,
		),
		TargetVault:    cfg.TargetVault,
		TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
		Frontmatter:    buildFrontmatter(pr, details, taskIDStr, cfg.Stage),
		Body:           buildTaskBody(pr, details, matchedSpecs),
	}
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func buildFrontmatter(
	pr PullRequest,
	details PRDetails,
	taskIDStr, stage string,
) agentlib.TaskFrontmatter {
	repo := pr.Owner + "/" + pr.Repo
	return agentlib.TaskFrontmatter{
		"task_type":       "dark-factory-implement",
		"assignee":        "github-dark-factory-agent",
		"phase":           "planning",
		"status":          "in_progress",
		"stage":           stage,
		"task_identifier": taskIDStr,
		"title": fmt.Sprintf(
			"Implement %s PR #%d at %s",
			repo,
			pr.Number,
			shortSHA(details.HeadSHA),
		),
		"repo":      repo,
		"clone_url": fmt.Sprintf("https://github.com/%s.git", repo),
		"ref":       details.HeadSHA,
		"pr_number": pr.Number,
		"branch":    details.Branch,
	}
}

// buildTaskBody is an operator-readable header only. The agent's data source is
// the clone of the draft PR branch, NOT this body.
func buildTaskBody(pr PullRequest, details PRDetails, matchedSpecs []string) string {
	return fmt.Sprintf(
		"# Dark-Factory Implement: %s/%s PR #%d\n\n"+
			"%s\n\n"+
			"Draft PR carrying an approved-not-completed dark-factory spec.\n\n"+
			"- **Branch:** %s\n"+
			"- **HEAD:** %s\n"+
			"- **Approved specs in diff:** %d\n",
		pr.Owner, pr.Repo, pr.Number,
		pr.HTMLURL,
		details.Branch,
		shortSHA(details.HeadSHA),
		len(matchedSpecs),
	)
}
