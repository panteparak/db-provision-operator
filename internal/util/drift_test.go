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

package util

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	dbopsv1alpha1 "github.com/db-provision-operator/api/v1alpha1"
)

var _ = Describe("ParseDriftRequeueInterval", func() {
	defaultInterval := 5 * time.Minute

	It("should return the parsed duration for a valid interval", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeDetect,
			Interval: "10s",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(10 * time.Second))
	})

	It("should return 5m for the default five-minute interval", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeDetect,
			Interval: "5m",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(5 * time.Minute))
	})

	It("should return the default interval for ignore mode", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeIgnore,
			Interval: "10s",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(defaultInterval))
	})

	It("should return the default interval for an empty interval string", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeDetect,
			Interval: "",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(defaultInterval))
	})

	It("should return the default interval for an invalid duration string", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeCorrect,
			Interval: "xyz",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(defaultInterval))
	})

	It("should work with correct mode and a valid short interval", func() {
		policy := dbopsv1alpha1.DriftPolicy{
			Mode:     dbopsv1alpha1.DriftModeCorrect,
			Interval: "30s",
		}
		Expect(ParseDriftRequeueInterval(policy, defaultInterval)).To(Equal(30 * time.Second))
	})
})
