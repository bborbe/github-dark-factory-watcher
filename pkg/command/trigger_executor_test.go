// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package command_test

import (
	"context"
	"time"

	task "github.com/bborbe/agent/command/task"
	taskmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/cqrs/base"
	cdb "github.com/bborbe/cqrs/cdb"
	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/mocks"
	"github.com/bborbe/github-dark-factory-watcher/pkg"
	"github.com/bborbe/github-dark-factory-watcher/pkg/command"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	lib "github.com/bborbe/maintainer"
)

const validPRURL = "https://github.com/bborbe/repo/pull/42"

// matchedSpec is the single approved-not-completed spec the candidate fixture
// exposes in the PR diff; the executor forwards it into BuildCreateCommand.
const matchedSpec = "specs/in-progress/spec.md"

// candidateDetails is the pulls/N-confirmed PRDetails of a dark-factory
// candidate: open, draft, with a resolvable head SHA.
var candidateDetails = pkg.PRDetails{
	HeadSHA:  "abc123",
	CloneURL: "https://github.com/bborbe/repo.git",
	Branch:   "dark-factory/foo",
	State:    "open",
	IsDraft:  true,
	Title:    "Feature: add support",
}

// executorTaskCfg mirrors the wiring the command consumer passes at runtime.
var executorTaskCfg = pkg.TaskConfig{
	Stage:       "dev",
	MaxSlugLen:  80,
	MaxTitleLen: 200,
}

// newMetrics returns a Metrics backed by a fresh registry so repeated
// construction in one process does not panic on duplicate registration.
func newMetrics() pkg.Metrics {
	return pkg.NewMetrics(prometheus.NewRegistry())
}

// configureCandidate wires the mock GitHub client so darkfactory.Evaluate
// returns Keep=true with MatchedSpecs=[matchedSpec].
func configureCandidate(gh *mocks.GitHubClient) {
	gh.GetPRDetailsReturns(candidateDetails, nil)
	gh.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
		if filePath == ".dark-factory.yaml" {
			return []byte("stage: dev\n"), nil
		}
		return []byte("---\napproved: yes\ncompleted:\n---\n# spec body\n"), nil
	}
	gh.ListDirReturns([]string{}, nil)
	gh.ListPRFilesReturns([]string{matchedSpec}, nil)
}

// outcome is the three-state exit-path classifier for the table-driven test.
type outcome int

const (
	outcomeSuccess outcome = iota
	outcomeSkipped
	outcomeWrappedErr
)

func mustParseEvent(cmd command.TriggerCommand) base.Event {
	evt, err := base.ParseEvent(context.Background(), cmd)
	Expect(err).NotTo(HaveOccurred())
	return evt
}

func newCommandObject(cmd command.TriggerCommand) cdb.CommandObject {
	return cdb.CommandObject{
		Command: base.Command{
			Operation: command.TriggerCommandOperation,
			Data:      mustParseEvent(cmd),
		},
		SchemaID: lib.GithubDarkFactoryV1SchemaID,
	}
}

var _ = Describe("NewTriggerCommandExecutor", func() {
	var (
		ctx          context.Context
		ghClient     *mocks.GitHubClient
		createSender *taskmocks.TaskCreateCommandSender
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		configureCandidate(ghClient)
	})

	DescribeTable("exit-path mapping",
		func(
			configure func(gh *mocks.GitHubClient),
			cmd command.TriggerCommand,
			expectOutcome outcome,
			expectDownstreamSent int,
		) {
			configure(ghClient)

			_, _, err := command.RunTrigger(
				ctx,
				nil,
				newCommandObject(cmd),
				ghClient, createSender, executorTaskCfg,
				newMetrics(),
				libtime.NewCurrentDateTime(),
			)

			switch expectOutcome {
			case outcomeSkipped:
				Expect(err).To(HaveOccurred(), "expected ErrCommandObjectSkipped")
				Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue(),
					"expected ErrCommandObjectSkipped, got %v", err)
			case outcomeWrappedErr:
				Expect(err).To(HaveOccurred(), "expected wrapped (transient) error")
				Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
					"transient errors must NOT be classified as Skipped, got %v", err)
			case outcomeSuccess:
				Expect(err).NotTo(HaveOccurred(), "unexpected error: %v", err)
			}
			Expect(createSender.SendCommandCallCount()).To(Equal(expectDownstreamSent),
				"downstream send count mismatch")
		},
		Entry("valid candidate → success + downstream sent",
			func(_ *mocks.GitHubClient) {},
			command.TriggerCommand{URL: validPRURL},
			outcomeSuccess, 1),
		Entry("invalid url (non-github) → skipped",
			func(_ *mocks.GitHubClient) {},
			command.TriggerCommand{
				URL: "https://bitbucket.example.com/projects/owner/repos/repo/pull-requests/1",
			},
			outcomeSkipped, 0),
		Entry("malformed / not-a-url → skipped",
			func(_ *mocks.GitHubClient) {},
			command.TriggerCommand{URL: "not-a-url"},
			outcomeSkipped, 0),
		Entry("non-candidate (not draft) → skipped",
			func(gh *mocks.GitHubClient) {
				details := candidateDetails
				details.IsDraft = false
				gh.GetPRDetailsReturns(details, nil)
			},
			command.TriggerCommand{URL: validPRURL},
			outcomeSkipped, 0),
		Entry("github 5xx (details) → wrapped err",
			func(gh *mocks.GitHubClient) {
				gh.GetPRDetailsReturns(
					pkg.PRDetails{},
					errors.Errorf(context.Background(), "github 5xx"),
				)
			},
			command.TriggerCommand{URL: validPRURL},
			outcomeWrappedErr, 0),
		Entry("evaluate infra err (non-404 GetContent) → wrapped err",
			func(gh *mocks.GitHubClient) {
				gh.GetContentStub = nil
				gh.GetContentReturns(nil, errors.Errorf(context.Background(), "github 503"))
			},
			command.TriggerCommand{URL: validPRURL},
			outcomeWrappedErr, 0),
	)

	It("kafka send err classifies transient (createSender fails)", func() {
		createSender.SendCommandReturns(errors.Errorf(ctx, "kafka send failed"))
		_, _, err := command.RunTrigger(
			ctx, nil, newCommandObject(command.TriggerCommand{URL: validPRURL}),
			ghClient, createSender, executorTaskCfg,
			newMetrics(), libtime.NewCurrentDateTime(),
		)
		Expect(err).To(HaveOccurred())
		Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse())
		Expect(createSender.SendCommandCallCount()).To(Equal(1))
	})
})

var _ = Describe("executor crash recovery", func() {
	// Proves at-least-once-via-idempotent-downstream: simulate a pod kill
	// mid-execution (context cancelled during gh fetch) and verify that on retry
	// the same downstream CreateTaskCommand is published exactly once.
	var (
		ctx          context.Context
		ghClient     *mocks.GitHubClient
		createSender *taskmocks.TaskCreateCommandSender
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		configureCandidate(ghClient)
	})

	It("a killed invocation can be retried and still publish exactly once", func() {
		killedCtx, cancel := context.WithCancel(ctx)
		ghClient.GetPRDetailsStub = func(c context.Context, _, _ string, _ int) (pkg.PRDetails, error) {
			cancel()
			return pkg.PRDetails{}, c.Err()
		}
		createSender.SendCommandStub = func(_ context.Context, _ task.CreateCommand) error {
			Fail("SendCommand must not be called during the killed invocation")
			return nil
		}

		commandObject := newCommandObject(command.TriggerCommand{URL: validPRURL})

		_, _, err := command.RunTrigger(
			killedCtx, nil, commandObject,
			ghClient, createSender, executorTaskCfg,
			newMetrics(), libtime.NewCurrentDateTime(),
		)
		Expect(err).To(HaveOccurred(),
			"killed invocation must return a transient error so Kafka redelivers")
		Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
			"killed invocation must NOT be classified as Skipped")
		Expect(createSender.SendCommandCallCount()).To(Equal(0))

		// Round 2: fresh context, fresh sender, deterministic github response.
		freshSender := new(taskmocks.TaskCreateCommandSender)
		ghClient.GetPRDetailsStub = nil
		configureCandidate(ghClient)

		_, _, err = command.RunTrigger(
			context.Background(), nil, commandObject,
			ghClient, freshSender, executorTaskCfg,
			newMetrics(), libtime.NewCurrentDateTime(),
		)
		Expect(err).NotTo(HaveOccurred(), "retry must succeed: %v", err)
		Expect(freshSender.SendCommandCallCount()).To(Equal(1),
			"retry must publish downstream exactly once")
	})
})

// force-true branch — exercises the executor's behaviour when
// TriggerCommand.Force is true. The executor must derive a salted task
// identifier via DeriveTaskIDForce (using a time-derived nonce from the injected
// libtime.CurrentDateTimeGetter) so the agent controller's vault-file dedup skip
// does not fire. The non-force path stays byte-identical to BuildCreateCommand.
var _ = Describe("force-true branch", func() {
	var (
		ctx             context.Context
		ghClient        *mocks.GitHubClient
		createSender    *taskmocks.TaskCreateCommandSender
		fakeNow         libtime.DateTime
		currentDateTime libtime.CurrentDateTimeGetter
	)

	BeforeEach(func() {
		ctx = context.Background()
		ghClient = new(mocks.GitHubClient)
		createSender = new(taskmocks.TaskCreateCommandSender)
		configureCandidate(ghClient)

		fakeNow = libtime.NewDateTime(2026, 6, 9, 12, 0, 0, 0, time.UTC)
		currentDateTime = libtime.CurrentDateTimeGetterFunc(
			func() libtime.DateTime { return fakeNow },
		)
	})

	// captureCreateCommand runs RunTrigger with the supplied command and returns
	// the CreateCommand the mock createSender received.
	captureCreateCommand := func(cmd command.TriggerCommand) task.CreateCommand {
		*createSender = taskmocks.TaskCreateCommandSender{}
		_, _, err := command.RunTrigger(
			ctx,
			nil,
			newCommandObject(cmd),
			ghClient, createSender, executorTaskCfg,
			newMetrics(),
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(createSender.SendCommandCallCount()).To(Equal(1),
			"executor must publish downstream exactly once for cmd=%+v", cmd)
		_, captured := createSender.SendCommandArgsForCall(0)
		return captured
	}

	// canonicalID is the (owner="bborbe", repo="repo", number=42, sha="abc123")
	// canonical TaskIdentifier — the non-force path's expected output.
	canonicalID := pkg.DeriveTaskID("bborbe", "repo", 42, "abc123").String()

	It("Force=true uses a salted identifier", func() {
		captured := captureCreateCommand(
			command.TriggerCommand{URL: validPRURL, Force: true},
		)
		Expect(string(captured.TaskIdentifier)).NotTo(Equal(canonicalID),
			"Force=true must produce a salted identifier different from the canonical one")
	})

	It("Force=false uses the canonical identifier", func() {
		captured := captureCreateCommand(
			command.TriggerCommand{URL: validPRURL, Force: false},
		)
		Expect(string(captured.TaskIdentifier)).To(Equal(canonicalID),
			"Force=false must produce the canonical (owner, repo, number, sha)-derived identifier")
	})

	It("Force=false produces the same CreateCommand as BuildCreateCommand", func() {
		captured := captureCreateCommand(
			command.TriggerCommand{URL: validPRURL, Force: false},
		)

		expected := pkg.BuildCreateCommand(
			pkg.PullRequest{
				Number:  42,
				Owner:   "bborbe",
				Repo:    "repo",
				Title:   candidateDetails.Title,
				HTMLURL: validPRURL,
				IsDraft: true,
			},
			candidateDetails,
			canonicalID,
			executorTaskCfg,
			[]string{matchedSpec},
		)

		Expect(string(captured.TaskIdentifier)).To(Equal(string(expected.TaskIdentifier)))
		Expect(captured.Title).To(Equal(expected.Title))
		Expect(captured.Frontmatter).To(Equal(expected.Frontmatter))
		Expect(captured.Body).To(Equal(expected.Body))
	})

	It("two Force=true triggers with different clocks produce different IDs", func() {
		first := captureCreateCommand(
			command.TriggerCommand{URL: validPRURL, Force: true},
		)
		fakeNow = fakeNow.Add(libtime.Microsecond)
		second := captureCreateCommand(
			command.TriggerCommand{URL: validPRURL, Force: true},
		)
		Expect(string(first.TaskIdentifier)).NotTo(Equal(string(second.TaskIdentifier)),
			"two Force=true invocations with a different clock must produce distinct identifiers")
	})

	It("Force=true increments the create label exactly once", func() {
		metrics := new(mocks.Metrics)
		*createSender = taskmocks.TaskCreateCommandSender{}
		_, _, err := command.RunTrigger(
			ctx,
			nil,
			newCommandObject(command.TriggerCommand{URL: validPRURL, Force: true}),
			ghClient, createSender, executorTaskCfg,
			metrics,
			currentDateTime,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(createSender.SendCommandCallCount()).To(Equal(1))

		Expect(metrics.IncPublishedCallCount()).To(Equal(1),
			"Force=true must increment published exactly once on success")
		Expect(metrics.IncPublishedArgsForCall(0)).To(Equal("create"),
			"Force=true must use the existing 'create' label")
		Expect(metrics.IncFilterSkippedCallCount()).To(Equal(0),
			"success path must not touch the filter-skipped counter")
	})
})
