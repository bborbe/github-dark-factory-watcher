// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/bborbe/errors"
	"github.com/bborbe/github-dark-factory-watcher/mocks"
	"github.com/bborbe/github-dark-factory-watcher/pkg/handler"
	libhttp "github.com/bborbe/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TriggerHandler", func() {
	var (
		ctx    context.Context
		sender *mocks.TriggerCommandSender
		h      http.Handler
	)

	BeforeEach(func() {
		ctx = context.Background()
		sender = new(mocks.TriggerCommandSender)
		h = libhttp.NewErrorHandler(handler.NewSinglePRTriggerHandler(sender))
	})

	DescribeTable(
		"error cases (400, no Kafka publish)",
		func(rawURL string) {
			sender.SendCommandReturns(nil) // should not be called
			req := httptest.NewRequest("POST", "/trigger?"+rawURL, nil)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusBadRequest))
			Expect(sender.SendCommandCallCount()).To(Equal(0),
				"SendCommand must not be called for invalid URL")
		},
		Entry("missing url returns 400", "foo=bar"),
		Entry("empty url returns 400", "url="),
		Entry("invalid url returns 400", "url=not-a-url"),
		Entry(
			"non-github platform returns 400",
			"url=https://bitbucket.org/owner/repo/pull-requests/1",
		),
	)

	Context("happy path: valid GitHub PR URL", func() {
		It("returns 202 with {status,url} body", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusAccepted))
			var body map[string]interface{}
			Expect(json.Unmarshal(resp.Body.Bytes(), &body)).To(Succeed())
			Expect(body["status"]).To(Equal("accepted"))
			Expect(body["url"]).To(Equal("https://github.com/bborbe/repo/pull/42"))
		})

		It("publishes exactly one TriggerCommand with the raw URL and Force=false", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			Expect(sender.SendCommandCallCount()).To(Equal(1))
			_, sentCmd := sender.SendCommandArgsForCall(0)
			Expect(sentCmd.URL).To(Equal("https://github.com/bborbe/repo/pull/42"))
			Expect(sentCmd.Force).To(BeFalse())
		})
	})

	Context("Kafka send failure", func() {
		BeforeEach(func() {
			sender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))
		})

		It("returns 502", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(resp.Code).To(Equal(http.StatusBadGateway))
		})
	})

	Context("force query param", func() {
		It("parses force=true", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42&force=true",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusAccepted))
			Expect(sender.SendCommandCallCount()).To(Equal(1))
			_, sentCmd := sender.SendCommandArgsForCall(0)
			Expect(sentCmd.Force).To(BeTrue())
		})

		It("parses force=false", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42&force=false",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(sender.SendCommandCallCount()).To(Equal(1))
			_, sentCmd := sender.SendCommandArgsForCall(0)
			Expect(sentCmd.Force).To(BeFalse())
		})

		It("defaults force to false when absent", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)
			Expect(sender.SendCommandCallCount()).To(Equal(1))
			_, sentCmd := sender.SendCommandArgsForCall(0)
			Expect(sentCmd.Force).To(BeFalse())
		})

		It("defaults force to false for garbage values", func() {
			req := httptest.NewRequest(
				"POST",
				"/trigger?url=https://github.com/bborbe/repo/pull/42&force=banana",
				nil,
			)
			resp := httptest.NewRecorder()
			h.ServeHTTP(resp, req)

			Expect(resp.Code).To(Equal(http.StatusAccepted))
			Expect(sender.SendCommandCallCount()).To(Equal(1))
			_, sentCmd := sender.SendCommandArgsForCall(0)
			Expect(sentCmd.Force).To(BeFalse())
		})
	})
})
