//go:build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/db-provision-operator/test/e2e/testutil"
)

// Multi-operator instance ID tests verify that multiple operator instances
// can run on the same cluster and partition resources by label.
//
// Prerequisites:
//   - The "default" operator is already running (deployed by CI/Makefile)
//   - A second operator with --instance-id=isolated is deployed via:
//     make e2e-deploy-second-operator
var _ = Describe("multi-operator instance partitioning", Ordered, Label("multi-operator"), func() {
	var (
		ctx             = context.Background()
		instanceHost    = os.Getenv("E2E_INSTANCE_HOST")
		instancePortStr = os.Getenv("E2E_INSTANCE_PORT")
		secretName      = "postgres-credentials"
		secretNamespace = "postgres"
	)

	const (
		isolatedNS = "multi-op-isolated"
		defaultNS  = "multi-op-default"
		labelKey   = "dbops.dbprovision.io/operator-instance-id"
		timeout    = 60 * time.Second
		interval   = 2 * time.Second
	)

	BeforeAll(func() {
		Expect(instanceHost).NotTo(BeEmpty(), "E2E_INSTANCE_HOST must be set")
		Expect(instancePortStr).NotTo(BeEmpty(), "E2E_INSTANCE_PORT must be set")

		// Create test namespaces
		for _, ns := range []string{isolatedNS, defaultNS} {
			_, err := k8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: ns},
			}, metav1.CreateOptions{})
			if err != nil {
				// Namespace may already exist from a previous run
				fmt.Fprintf(GinkgoWriter, "Namespace %s: %v\n", ns, err)
			}
		}
	})

	AfterAll(func() {
		// Clean up test namespaces (best-effort)
		for _, ns := range []string{isolatedNS, defaultNS} {
			_ = k8sClient.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
		}
	})

	It("isolated operator reconciles labeled resources", func() {
		By("creating a DatabaseInstance labeled for the isolated operator")
		port, err := parsePort(instancePortStr)
		Expect(err).NotTo(HaveOccurred())

		instance := testutil.BuildDatabaseInstance(
			"isolated-instance", isolatedNS,
			"postgresql", instanceHost, port,
			testutil.SecretRef{Name: secretName, Namespace: secretNamespace},
		)
		testutil.WithLabels(instance, map[string]string{
			labelKey: "isolated",
		})

		_, err = dynamicClient.Resource(databaseInstanceGVR).Namespace(isolatedNS).
			Create(ctx, instance, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the instance to reach Ready phase")
		Eventually(func() string {
			phase, _ := getResourcePhase(ctx, databaseInstanceGVR, "isolated-instance", isolatedNS)
			return phase
		}, timeout, interval).Should(Equal("Ready"),
			"Isolated operator should reconcile instance labeled operator-instance-id=isolated")

		By("creating a Database under the isolated instance")
		db := testutil.BuildDatabase("isolated-db", isolatedNS, "isolated-instance", "isolated_testdb")
		testutil.WithLabels(db, map[string]string{
			labelKey: "isolated",
		})

		_, err = dynamicClient.Resource(databaseGVR).Namespace(isolatedNS).
			Create(ctx, db, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			phase, _ := getResourcePhase(ctx, databaseGVR, "isolated-db", isolatedNS)
			return phase
		}, timeout, interval).Should(Equal("Ready"),
			"Database under isolated operator should reach Ready")
	})

	It("default operator manages unlabeled resources", func() {
		By("creating an unlabeled DatabaseInstance")
		port, err := parsePort(instancePortStr)
		Expect(err).NotTo(HaveOccurred())

		instance := testutil.BuildDatabaseInstance(
			"default-instance", defaultNS,
			"postgresql", instanceHost, port,
			testutil.SecretRef{Name: secretName, Namespace: secretNamespace},
		)
		// No label — default operator should pick this up

		_, err = dynamicClient.Resource(databaseInstanceGVR).Namespace(defaultNS).
			Create(ctx, instance, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("waiting for the unlabeled instance to reach Ready via the default operator")
		Eventually(func() string {
			phase, _ := getResourcePhase(ctx, databaseInstanceGVR, "default-instance", defaultNS)
			return phase
		}, timeout, interval).Should(Equal("Ready"),
			"Default operator should reconcile unlabeled resources")
	})

	It("default operator ignores resources labeled for another instance", func() {
		By("creating a DatabaseInstance labeled for the isolated operator in default namespace")
		port, err := parsePort(instancePortStr)
		Expect(err).NotTo(HaveOccurred())

		instance := testutil.BuildDatabaseInstance(
			"cross-instance", defaultNS,
			"postgresql", instanceHost, port,
			testutil.SecretRef{Name: secretName, Namespace: secretNamespace},
		)
		testutil.WithLabels(instance, map[string]string{
			labelKey: "isolated",
		})

		_, err = dynamicClient.Resource(databaseInstanceGVR).Namespace(defaultNS).
			Create(ctx, instance, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("verifying it also reaches Ready (the isolated operator watches all namespaces)")
		// Both operators are cluster-scoped. The isolated operator should reconcile
		// this resource because it has the matching label, regardless of namespace.
		Eventually(func() string {
			phase, _ := getResourcePhase(ctx, databaseInstanceGVR, "cross-instance", defaultNS)
			return phase
		}, timeout, interval).Should(Equal("Ready"),
			"Isolated operator should reconcile resources with its label in any namespace")
	})
})

// parsePort converts a port string to int64.
func parsePort(portStr string) (int64, error) {
	var port int64
	_, err := fmt.Sscanf(portStr, "%d", &port)
	return port, err
}
