/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logging

import (
	"context"
	"regexp"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("GenerateID", func() {
	It("should return an 8-character hex string", func() {
		id := GenerateID()
		Expect(id).To(HaveLen(8))
		Expect(id).To(MatchRegexp("^[0-9a-f]{8}$"))
	})

	It("should produce unique values on successive calls", func() {
		ids := make(map[string]struct{}, 100)
		for i := 0; i < 100; i++ {
			id := GenerateID()
			ids[id] = struct{}{}
		}
		Expect(ids).To(HaveLen(100))
	})

	It("should only contain lowercase hex characters", func() {
		for i := 0; i < 50; i++ {
			id := GenerateID()
			Expect(regexp.MustCompile(`^[0-9a-f]+$`).MatchString(id)).To(BeTrue())
		}
	})
})

var _ = Describe("IDFromContext", func() {
	It("should return empty string from empty context", func() {
		ctx := context.Background()
		Expect(IDFromContext(ctx)).To(BeEmpty())
	})

	It("should round-trip a reconcileID through context", func() {
		ctx := context.WithValue(context.Background(), reconcileIDKey{}, "abc12345")
		Expect(IDFromContext(ctx)).To(Equal("abc12345"))
	})
})

var _ = Describe("withReconcileID middleware", func() {
	var baseCtx context.Context

	BeforeEach(func() {
		// Set up a base logger in context, mimicking what controller-runtime does
		baseLog := zap.New(zap.UseDevMode(true))
		logf.SetLogger(baseLog)
		baseCtx = logr.NewContext(context.Background(), baseLog)
	})

	It("should inject reconcileID into context before calling inner reconciler", func() {
		var capturedID string

		inner := reconcile.Func(func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			capturedID = IDFromContext(ctx)
			log := logf.FromContext(ctx)
			log.Info("test log from inner reconciler")
			return ctrl.Result{}, nil
		})

		middleware := &withReconcileID{inner: inner}

		result, err := middleware.Reconcile(baseCtx, ctrl.Request{})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{}))

		// Verify reconcileID was injected
		Expect(capturedID).To(HaveLen(8))
		Expect(capturedID).To(MatchRegexp("^[0-9a-f]{8}$"))
	})

	It("should generate a different ID for each reconciliation", func() {
		var ids []string

		inner := reconcile.Func(func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			ids = append(ids, IDFromContext(ctx))
			return ctrl.Result{}, nil
		})

		middleware := &withReconcileID{inner: inner}

		for i := 0; i < 10; i++ {
			_, _ = middleware.Reconcile(baseCtx, ctrl.Request{})
		}

		Expect(ids).To(HaveLen(10))
		// All IDs should be unique
		uniqueIDs := make(map[string]struct{})
		for _, id := range ids {
			uniqueIDs[id] = struct{}{}
		}
		Expect(uniqueIDs).To(HaveLen(10))
	})

	It("should propagate the result and error from the inner reconciler", func() {
		expectedResult := ctrl.Result{RequeueAfter: 42}
		expectedErr := context.DeadlineExceeded

		inner := reconcile.Func(func(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
			return expectedResult, expectedErr
		})

		middleware := &withReconcileID{inner: inner}

		result, err := middleware.Reconcile(baseCtx, ctrl.Request{})
		Expect(result).To(Equal(expectedResult))
		Expect(err).To(MatchError(expectedErr))
	})
})
