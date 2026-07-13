// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package darkfactory_test

import (
	"context"
	stderrors "errors"

	"github.com/bborbe/github-dark-factory-watcher/mocks"
	"github.com/bborbe/github-dark-factory-watcher/pkg/darkfactory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var errNotFound = stderrors.New("404 not found")

const (
	approvedSpec  = "---\nstatus: verifying\napproved: \"2026-06-30T11:23:08Z\"\n---\n\n## Summary\n"
	completedSpec = "---\napproved: \"2026-06-30T11:23:08Z\"\ncompleted: \"2026-07-01T09:00:00Z\"\n---\n"
	draftSpec     = "---\nstatus: draft\n---\n\n## Summary\n"
)

var _ = Describe("Evaluate", func() {
	var (
		ctx    context.Context
		reader *mocks.ContentReader
		in     darkfactory.Input
	)

	BeforeEach(func() {
		ctx = context.Background()
		reader = new(mocks.ContentReader)
		reader.IsNotFoundStub = func(err error) bool { return stderrors.Is(err, errNotFound) }
		in = darkfactory.Input{
			Owner:   "bborbe",
			Repo:    "widget",
			Number:  42,
			HeadSHA: "deadbeef",
			IsDraft: true,
			State:   "open",
		}
		// Defaults for the happy path — overridden per test.
		reader.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
			switch filePath {
			case ".dark-factory.yaml":
				return []byte("release:\n  autoRelease: false\n"), nil
			case "specs/in-progress/001-feature.md":
				return []byte(approvedSpec), nil
			default:
				return nil, errNotFound
			}
		}
		reader.ListDirReturns(nil, errNotFound) // no prompts/in-progress
		reader.ListPRFilesReturns([]string{
			"specs/in-progress/001-feature.md",
			"pkg/widget.go",
		}, nil)
	})

	It("keeps a draft PR with an approved-not-completed spec in the diff", func() {
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Keep).To(BeTrue())
		Expect(result.Reason).To(BeEmpty())
	})

	It("skips a non-draft PR", func() {
		in.IsDraft = false
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Keep).To(BeFalse())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNotDraft))
	})

	It("skips a closed PR", func() {
		in.State = "closed"
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNotOpen))
	})

	It("skips when .dark-factory.yaml is absent", func() {
		reader.GetContentStub = func(_ context.Context, _, _, _, _ string) ([]byte, error) {
			return nil, errNotFound
		}
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNoDarkFactoryYML))
	})

	It("returns the error on a non-404 config read failure", func() {
		boom := stderrors.New("500 server error")
		reader.GetContentStub = func(_ context.Context, _, _, _, _ string) ([]byte, error) {
			return nil, boom
		}
		_, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).To(HaveOccurred())
	})

	It("skips when prompts/in-progress still holds a *.md (self-trigger)", func() {
		reader.ListDirReturns([]string{"prompts/in-progress/001-feature.md"}, nil)
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonPromptsInFlight))
	})

	It("skips when no spec under specs/in-progress is in the diff", func() {
		reader.ListPRFilesReturns([]string{"pkg/widget.go", "README.md"}, nil)
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNoSpecInDiff))
	})

	It("skips when the touched spec is already completed", func() {
		reader.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
			switch filePath {
			case ".dark-factory.yaml":
				return []byte("x: y\n"), nil
			case "specs/in-progress/001-feature.md":
				return []byte(completedSpec), nil
			default:
				return nil, errNotFound
			}
		}
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNoSpecInDiff))
	})

	It("skips when the touched spec is not yet approved", func() {
		reader.GetContentStub = func(_ context.Context, _, _, filePath, _ string) ([]byte, error) {
			switch filePath {
			case ".dark-factory.yaml":
				return []byte("x: y\n"), nil
			case "specs/in-progress/001-feature.md":
				return []byte(draftSpec), nil
			default:
				return nil, errNotFound
			}
		}
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNoSpecInDiff))
	})

	It("ignores a diff-touched spec that is absent at head (404)", func() {
		reader.ListPRFilesReturns([]string{"specs/in-progress/999-deleted.md"}, nil)
		result, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Reason).To(Equal(darkfactory.ReasonNoSpecInDiff))
	})

	It("returns the error when listing PR files fails", func() {
		reader.ListPRFilesReturns(nil, stderrors.New("api down"))
		_, err := darkfactory.Evaluate(ctx, reader, in)
		Expect(err).To(HaveOccurred())
	})
})
