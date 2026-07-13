// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	stderrors "errors"
	"os"
	"path/filepath"
	"time"

	task "github.com/bborbe/agent/command/task"
	taskmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/github-dark-factory-watcher/mocks"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/bborbe/github-dark-factory-watcher/pkg/filter"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var errNotFound = stderrors.New("404 not found")

const approvedSpecMD = "---\nstatus: verifying\napproved: \"2026-06-30T11:23:08Z\"\n---\n\n## Summary\n"

func newTestWatcher(
	ghClient pkg.GitHubClient,
	createSender task.CreateCommandSender,
	cursorPath string,
	startTime libtime.DateTime,
	fakeMetrics *mocks.Metrics,
	scopeFilter filter.TaskCreationFilter,
) pkg.Watcher {
	return pkg.NewWatcher(
		ghClient,
		createSender,
		fakeMetrics,
		cursorPath,
		startTime,
		"bborbe",
		scopeFilter,
		pkg.TaskConfig{
			Stage:       "dev",
			MaxSlugLen:  pkg.DefaultMaxSlugLen,
			MaxTitleLen: pkg.DefaultMaxTitleLen,
		},
	)
}

// configureCandidate wires a fake GitHubClient so a single PR passes every
// dark-factory gate (open draft, .dark-factory.yaml present, approved-not-
// completed spec in the diff, no in-flight prompts).
func configureCandidate(ghClient *mocks.GitHubClient, pr pkg.PullRequest) {
	ghClient.SearchPRsReturns(pkg.SearchResult{
		PullRequests:  []pkg.PullRequest{pr},
		HasNextPage:   false,
		RateRemaining: 100,
	}, nil)
	ghClient.GetPRDetailsReturns(pkg.PRDetails{
		HeadSHA:  "abc123",
		CloneURL: "https://github.com/bborbe/widget.git",
		Branch:   "dark-factory/thing",
		State:    "open",
		IsDraft:  true,
		Title:    "feat: thing",
	}, nil)
	ghClient.IsNotFoundStub = func(err error) bool { return stderrors.Is(err, errNotFound) }
	ghClient.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
		switch filePath {
		case ".dark-factory.yaml":
			return []byte("release:\n  autoRelease: false\n"), nil
		case "specs/in-progress/001-thing.md":
			return []byte(approvedSpecMD), nil
		default:
			return nil, errNotFound
		}
	}
	ghClient.ListDirReturns(nil, errNotFound)
	ghClient.ListPRFilesReturns([]string{"specs/in-progress/001-thing.md", "pkg/thing.go"}, nil)
}

func candidatePR() pkg.PullRequest {
	return pkg.PullRequest{
		Number:    42,
		Owner:     "bborbe",
		Repo:      "widget",
		Title:     "feat: thing",
		HTMLURL:   "https://github.com/bborbe/widget/pull/42",
		IsDraft:   true,
		UpdatedAt: libtime.DateTime(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)),
	}
}

var _ = Describe("pkg.Watcher", func() {
	var (
		ctx          context.Context
		cancel       context.CancelFunc
		ghClient     *mocks.GitHubClient
		createSender *taskmocks.TaskCreateCommandSender
		fakeMetrics  *mocks.Metrics
		tmpDir       string
		cursorPath   string
		startTime    libtime.DateTime
		allowAll     filter.TaskCreationFilter
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		fakeMetrics = new(mocks.Metrics)
		startTime = libtime.DateTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
		allowAll = filter.TaskCreationFilters{}
		var err error
		tmpDir, err = os.MkdirTemp("", "watcher-test-*")
		Expect(err).NotTo(HaveOccurred())
		cursorPath = filepath.Join(tmpDir, "cursor.json")
	})

	AfterEach(func() {
		cancel()
		_ = os.RemoveAll(tmpDir) // #nosec G104 -- best-effort temp dir cleanup
	})

	Describe("No PRs returned", func() {
		It("saves the cursor and records a successful poll", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{RateRemaining: 100}, nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			_, err := os.Stat(cursorPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeMetrics.IncPollCycleArgsForCall(0)).To(Equal("success"))
		})
	})

	Describe("Candidate draft PR with an approved-not-completed spec in the diff", func() {
		It("publishes a dark-factory-implement CreateTaskCommand", func() {
			configureCandidate(ghClient, candidatePR())
			createSender.SendCommandReturns(nil)

			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())

			Expect(createSender.SendCommandCallCount()).To(Equal(1))
			_, cmd := createSender.SendCommandArgsForCall(0)
			Expect(cmd.Frontmatter["task_type"]).To(Equal("dark-factory-implement"))
			Expect(cmd.Frontmatter["assignee"]).To(Equal("github-dark-factory-agent"))
			Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
			Expect(cmd.Frontmatter["ref"]).To(Equal("abc123"))
			Expect(cmd.Frontmatter["branch"]).To(Equal("dark-factory/thing"))
			Expect(cmd.Frontmatter["pr_number"]).To(Equal(42))
			Expect(
				cmd.Title,
			).To(HavePrefix("Dark Factory Implement github - bborbe-widget - 42 - abc123"))
			Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("create"))
		})
	})

	Describe("Same (PR, SHA) on a second poll", func() {
		It("publishes nothing the second time", func() {
			configureCandidate(ghClient, candidatePR())
			createSender.SendCommandReturns(nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(1))

			createSender2 := new(taskmocks.TaskCreateCommandSender)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender2.SendCommandCallCount()).To(Equal(0))
		})
	})

	Describe("New commit (different head SHA)", func() {
		It("publishes a new command for the new SHA", func() {
			configureCandidate(ghClient, candidatePR())
			createSender.SendCommandReturns(nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			_, cmd1 := createSender.SendCommandArgsForCall(0)

			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA: "def456", CloneURL: "https://github.com/bborbe/widget.git",
				Branch: "dark-factory/thing", State: "open", IsDraft: true, Title: "feat: thing",
			}, nil)
			ghClient.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
				if filePath == ".dark-factory.yaml" {
					return []byte("x: y\n"), nil
				}
				if filePath == "specs/in-progress/001-thing.md" {
					return []byte(approvedSpecMD), nil
				}
				return nil, errNotFound
			}
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender2.SendCommandCallCount()).To(Equal(1))
			_, cmd2 := createSender2.SendCommandArgsForCall(0)
			Expect(string(cmd2.TaskIdentifier)).NotTo(Equal(string(cmd1.TaskIdentifier)))
			Expect(cmd2.Frontmatter["ref"]).To(Equal("def456"))
		})
	})

	Describe("Non-draft PR", func() {
		It("is skipped as not_draft", func() {
			configureCandidate(ghClient, candidatePR())
			ghClient.GetPRDetailsReturns(pkg.PRDetails{
				HeadSHA: "abc123", State: "open", IsDraft: false, Branch: "b",
			}, nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("not_draft"))
		})
	})

	Describe("No spec under specs/in-progress in the diff", func() {
		It("is skipped as no_spec_in_diff", func() {
			configureCandidate(ghClient, candidatePR())
			ghClient.ListPRFilesReturns([]string{"pkg/thing.go", "README.md"}, nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("no_spec_in_diff"))
		})
	})

	Describe("Out-of-scope repo", func() {
		It("is skipped as out_of_scope before any GitHub detail call", func() {
			configureCandidate(ghClient, candidatePR())
			scope := filter.TaskCreationFilters{
				filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/other"}),
			}
			w := newTestWatcher(ghClient, createSender, cursorPath, startTime, fakeMetrics, scope)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(ghClient.GetPRDetailsCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("out_of_scope"))
		})
	})

	Describe("GitHub search error", func() {
		It("records github_error, does not save the cursor", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{}, stderrors.New("network timeout"))
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			_, statErr := os.Stat(cursorPath)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
			Expect(fakeMetrics.IncPollCycleArgsForCall(0)).To(Equal("github_error"))
		})
	})

	Describe("GetPRDetails error", func() {
		It("skips as details_error, does not advance the cursor", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests: []pkg.PullRequest{candidatePR()}, RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(pkg.PRDetails{}, stderrors.New("api error"))
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("details_error"))
			cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(cursor.LastUpdatedAt).To(Equal(startTime))
		})
	})

	Describe("Empty head SHA", func() {
		It("skips as empty_sha", func() {
			ghClient.SearchPRsReturns(pkg.SearchResult{
				PullRequests: []pkg.PullRequest{candidatePR()}, RateRemaining: 100,
			}, nil)
			ghClient.GetPRDetailsReturns(
				pkg.PRDetails{HeadSHA: "", State: "open", IsDraft: true},
				nil,
			)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("empty_sha"))
		})
	})

	Describe("Evaluate error (non-404 config read failure)", func() {
		It("skips as evaluate_error, does not advance the cursor", func() {
			configureCandidate(ghClient, candidatePR())
			ghClient.GetContentStub = func(_ context.Context, _, _, _, _ string) ([]byte, error) {
				return nil, stderrors.New("500 server error")
			}
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(createSender.SendCommandCallCount()).To(Equal(0))
			Expect(fakeMetrics.IncFilterSkippedArgsForCall(0)).To(Equal("evaluate_error"))
		})
	})

	Describe("Kafka publish fails", func() {
		It("records error and does not add the task to the cursor (retries next poll)", func() {
			configureCandidate(ghClient, candidatePR())
			createSender.SendCommandReturns(stderrors.New("kafka unavailable"))
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			Expect(fakeMetrics.IncPublishedArgsForCall(0)).To(Equal("error"))

			taskID := pkg.DeriveTaskID("bborbe", "widget", 42, "abc123").String()
			cursor, err := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(err).NotTo(HaveOccurred())
			Expect(cursor.HeadSHAs).NotTo(HaveKey(taskID))
		})
	})

	Describe("Closed PR pruned from cursor", func() {
		It("removes the closed PR's task ID after the next poll", func() {
			configureCandidate(ghClient, candidatePR())
			createSender.SendCommandReturns(nil)
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).NotTo(HaveOccurred())
			taskID := pkg.DeriveTaskID("bborbe", "widget", 42, "abc123").String()
			cursor, _ := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(cursor.HeadSHAs).To(HaveKey(taskID))

			// Second poll: PR no longer returned by search.
			ghClient.SearchPRsReturns(pkg.SearchResult{RateRemaining: 100}, nil)
			createSender2 := new(taskmocks.TaskCreateCommandSender)
			w2 := newTestWatcher(
				ghClient,
				createSender2,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
			cursor2, _ := pkg.LoadCursor(ctx, cursorPath, startTime)
			Expect(cursor2.HeadSHAs).NotTo(HaveKey(taskID))
		})
	})

	Describe("Cursor load error (unreadable file)", func() {
		It("returns a non-nil error", func() {
			if os.Getuid() == 0 {
				Skip("running as root, skipping permission test")
			}
			Expect(os.WriteFile(cursorPath, []byte("{}"), 0600)).To(Succeed())
			Expect(os.Chmod(cursorPath, 0000)).To(Succeed())
			defer func() { _ = os.Chmod(cursorPath, 0600) }()
			w := newTestWatcher(
				ghClient,
				createSender,
				cursorPath,
				startTime,
				fakeMetrics,
				allowAll,
			)
			Expect(w.Poll(ctx)).To(HaveOccurred())
		})
	})
})

var _ = Describe("BuildCreateCommand", func() {
	It("emits the exact dark-factory-implement frontmatter contract", func() {
		pr := pkg.PullRequest{
			Number:  7,
			Owner:   "bborbe",
			Repo:    "widget",
			Title:   "feat: add thing",
			HTMLURL: "https://github.com/bborbe/widget/pull/7",
		}
		details := pkg.PRDetails{
			HeadSHA: "abcdef1234567890",
			Branch:  "dark-factory/add-thing",
			State:   "open",
			IsDraft: true,
			Title:   "feat: add thing",
		}
		taskIDStr := "00000000-0000-0000-0000-000000000001"

		cmd := pkg.BuildCreateCommand(pr, details, taskIDStr, pkg.TaskConfig{
			Stage:       "prod",
			MaxSlugLen:  pkg.DefaultMaxSlugLen,
			MaxTitleLen: pkg.DefaultMaxTitleLen,
			TargetVault: "agent",
		}, []string{"specs/in-progress/001-thing.md"})

		Expect(cmd.TargetVault).To(Equal("agent"))
		Expect(string(cmd.TaskIdentifier)).To(Equal(taskIDStr))
		fm := cmd.Frontmatter
		Expect(fm["task_type"]).To(Equal("dark-factory-implement"))
		Expect(fm["assignee"]).To(Equal("github-dark-factory-agent"))
		Expect(fm["phase"]).To(Equal("planning"))
		Expect(fm["status"]).To(Equal("in_progress"))
		Expect(fm["stage"]).To(Equal("prod"))
		Expect(fm["task_identifier"]).To(Equal(taskIDStr))
		Expect(fm["title"]).To(Equal("Implement bborbe/widget PR #7 at abcdef1"))
		Expect(fm["repo"]).To(Equal("bborbe/widget"))
		Expect(fm["clone_url"]).To(Equal("https://github.com/bborbe/widget.git"))
		Expect(fm["ref"]).To(Equal("abcdef1234567890"))
		Expect(fm["pr_number"]).To(Equal(7))
		Expect(fm["branch"]).To(Equal("dark-factory/add-thing"))
		Expect(cmd.Body).To(ContainSubstring("https://github.com/bborbe/widget/pull/7"))
		Expect(cmd.Body).To(ContainSubstring("Approved specs in diff:** 1"))
	})
})
