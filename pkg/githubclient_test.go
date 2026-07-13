// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pkg_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"time"

	"github.com/bborbe/github-dark-factory-watcher/pkg"
	libtime "github.com/bborbe/time"
	gogithub "github.com/google/go-github/v62/github"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var fixedNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

var _ = Describe("pkg.GitHubClient", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
	})

	buildClient := func(server *httptest.Server) pkg.GitHubClient {
		ghc := gogithub.NewClient(server.Client())
		baseURL, _ := url.Parse(server.URL + "/")
		ghc.BaseURL = baseURL
		return pkg.NewForTest(ghc)
	}

	Describe("SearchPRs", func() {
		It("returns both PRs with correct fields", func() {
			resetAt := fixedNow.Add(time.Hour).Unix()
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/search/issues"))
					w.Header().Set("X-RateLimit-Remaining", "4999")
					w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{
						"total_count": 2,
						"incomplete_results": false,
						"items": [
							{
								"id": 1001, "number": 42, "title": "Fix bug",
								"html_url": "https://github.com/owner/repo/pull/42",
								"repository_url": "https://api.github.com/repos/owner/repo",
								"draft": true, "updated_at": "2026-01-01T00:00:00Z"
							},
							{
								"id": 1002, "number": 43, "title": "Add feature",
								"html_url": "https://github.com/owner/repo/pull/43",
								"repository_url": "https://api.github.com/repos/owner/repo",
								"draft": false, "updated_at": "2026-01-02T00:00:00Z"
							}
						]
					}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			result, err := client.SearchPRs(
				ctx,
				"owner",
				libtime.DateTime(fixedNow.Add(-24*time.Hour)),
				1,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PullRequests).To(HaveLen(2))
			Expect(result.HasNextPage).To(BeFalse())
			Expect(result.RateRemaining).To(Equal(4999))

			pr := result.PullRequests[0]
			Expect(pr.GlobalID).To(Equal(int64(1001)))
			Expect(pr.Number).To(Equal(42))
			Expect(pr.Owner).To(Equal("owner"))
			Expect(pr.Repo).To(Equal("repo"))
			Expect(pr.Title).To(Equal("Fix bug"))
			Expect(pr.HTMLURL).To(Equal("https://github.com/owner/repo/pull/42"))
			Expect(pr.IsDraft).To(BeTrue())
			Expect(result.PullRequests[1].IsDraft).To(BeFalse())
		})

		It("returns an error on HTTP failure", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					fmt.Fprintf(w, `{"message":"Bad credentials"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			_, err := client.SearchPRs(ctx, "org", libtime.DateTime(fixedNow.Add(-time.Hour)), 1)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetPRDetails", func() {
		It("returns head SHA, clone URL, branch, state and draft flag", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/pulls/42"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{
						"number": 42, "state": "open", "draft": true, "title": "Fix bug",
						"head": {
							"sha": "abc123def456abc123def456abc123def456abc1",
							"ref": "dark-factory/fix-bug",
							"repo": {"clone_url": "https://github.com/owner/repo.git"}
						}
					}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			details, err := client.GetPRDetails(ctx, "owner", "repo", 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(details.HeadSHA).To(Equal("abc123def456abc123def456abc123def456abc1"))
			Expect(details.CloneURL).To(Equal("https://github.com/owner/repo.git"))
			Expect(details.Branch).To(Equal("dark-factory/fix-bug"))
			Expect(details.State).To(Equal("open"))
			Expect(details.IsDraft).To(BeTrue())
			Expect(details.Title).To(Equal("Fix bug"))
		})

		It("returns an error on HTTP failure", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, `{"message":"Not Found"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			_, err := client.GetPRDetails(ctx, "owner", "repo", 99)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ListPRFiles", func() {
		It("returns the filenames across paginated pages", func() {
			var serverURL string
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/pulls/42/files"))
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Query().Get("page") == "2" {
						fmt.Fprintf(w, `[{"filename":"pkg/two.go"}]`)
						return
					}
					w.Header().
						Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls/42/files?page=2>; rel="next"`, serverURL))
					fmt.Fprintf(
						w,
						`[{"filename":"specs/in-progress/001.md"},{"filename":"pkg/one.go"}]`,
					)
				}),
			)
			serverURL = server.URL
			defer server.Close()

			client := buildClient(server)
			files, err := client.ListPRFiles(ctx, "owner", "repo", 42)
			Expect(err).NotTo(HaveOccurred())
			Expect(
				files,
			).To(Equal([]string{"specs/in-progress/001.md", "pkg/one.go", "pkg/two.go"}))
		})
	})

	Describe("GetContent", func() {
		It("returns the decoded file bytes at ref", func() {
			payload := "release:\n  autoRelease: false\n"
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/contents/.dark-factory.yaml"))
					Expect(r.URL.Query().Get("ref")).To(Equal("deadbeef"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(
						w,
						`{"type":"file","name":".dark-factory.yaml","path":".dark-factory.yaml","encoding":"base64","content":%q}`,
						base64.StdEncoding.EncodeToString([]byte(payload)),
					)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			content, err := client.GetContent(
				ctx,
				"owner",
				"repo",
				".dark-factory.yaml",
				"deadbeef",
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(Equal(payload))
		})

		It("returns a 404 error that IsNotFound recognizes", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, `{"message":"Not Found"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			_, err := client.GetContent(ctx, "owner", "repo", "missing.yaml", "deadbeef")
			Expect(err).To(HaveOccurred())
			Expect(client.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("ListDir", func() {
		It("returns the entry paths of a directory at ref", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					Expect(r.URL.Path).To(Equal("/repos/owner/repo/contents/prompts/in-progress"))
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(
						w,
						`[{"type":"file","name":"001.md","path":"prompts/in-progress/001.md"}]`,
					)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			entries, err := client.ListDir(ctx, "owner", "repo", "prompts/in-progress", "deadbeef")
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(Equal([]string{"prompts/in-progress/001.md"}))
		})

		It("returns a 404 error that IsNotFound recognizes for a missing dir", func() {
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, `{"message":"Not Found"}`)
				}),
			)
			defer server.Close()

			client := buildClient(server)
			_, err := client.ListDir(ctx, "owner", "repo", "prompts/in-progress", "deadbeef")
			Expect(err).To(HaveOccurred())
			Expect(client.IsNotFound(err)).To(BeTrue())
		})
	})

	Describe("IsNotFound", func() {
		It("returns false for a nil or non-GitHub error", func() {
			client := pkg.NewGitHubClient(nil)
			Expect(client.IsNotFound(nil)).To(BeFalse())
			Expect(client.IsNotFound(context.Canceled)).To(BeFalse())
		})
	})
})
