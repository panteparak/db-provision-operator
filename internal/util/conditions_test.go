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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Conditions", func() {
	Describe("SetCondition", func() {
		var conditions []metav1.Condition

		BeforeEach(func() {
			conditions = []metav1.Condition{}
		})

		Context("when adding a new condition", func() {
			It("should add the condition to the list", func() {
				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Resource is ready")

				Expect(conditions).To(HaveLen(1))
				Expect(conditions[0].Type).To(Equal(ConditionTypeReady))
				Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
				Expect(conditions[0].Reason).To(Equal(ReasonReconcileSuccess))
				Expect(conditions[0].Message).To(Equal("Resource is ready"))
				Expect(conditions[0].LastTransitionTime.IsZero()).To(BeFalse())
			})

			It("should set LastTransitionTime", func() {
				beforeTime := time.Now().Add(-1 * time.Second)
				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Ready")
				afterTime := time.Now().Add(1 * time.Second)

				Expect(conditions[0].LastTransitionTime.Time.After(beforeTime)).To(BeTrue())
				Expect(conditions[0].LastTransitionTime.Time.Before(afterTime)).To(BeTrue())
			})

			It("should add multiple different conditions", func() {
				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Ready")
				SetCondition(&conditions, ConditionTypeConnected, metav1.ConditionTrue, ReasonConnectionSuccess, "Connected")
				SetCondition(&conditions, ConditionTypeHealthy, metav1.ConditionTrue, ReasonHealthCheckPassed, "Healthy")

				Expect(conditions).To(HaveLen(3))
			})
		})

		Context("when updating an existing condition", func() {
			BeforeEach(func() {
				conditions = []metav1.Condition{
					{
						Type:               ConditionTypeReady,
						Status:             metav1.ConditionFalse,
						Reason:             ReasonReconciling,
						Message:            "Reconciling",
						LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
						ObservedGeneration: 1,
					},
				}
			})

			It("should update status when changed", func() {
				originalTime := conditions[0].LastTransitionTime

				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Ready")

				Expect(conditions).To(HaveLen(1))
				Expect(conditions[0].Status).To(Equal(metav1.ConditionTrue))
				Expect(conditions[0].LastTransitionTime.After(originalTime.Time)).To(BeTrue())
			})

			It("should update reason when changed", func() {
				originalTime := conditions[0].LastTransitionTime

				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileFailed, "Reconciling")

				Expect(conditions).To(HaveLen(1))
				Expect(conditions[0].Reason).To(Equal(ReasonReconcileFailed))
				Expect(conditions[0].LastTransitionTime.After(originalTime.Time)).To(BeTrue())
			})

			It("should update message when changed", func() {
				originalTime := conditions[0].LastTransitionTime

				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionFalse, ReasonReconciling, "Still reconciling")

				Expect(conditions).To(HaveLen(1))
				Expect(conditions[0].Message).To(Equal("Still reconciling"))
				Expect(conditions[0].LastTransitionTime.After(originalTime.Time)).To(BeTrue())
			})

			It("should preserve ObservedGeneration when updating", func() {
				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Ready")

				Expect(conditions[0].ObservedGeneration).To(Equal(int64(1)))
			})
		})

		Context("when condition is unchanged", func() {
			It("should not update LastTransitionTime", func() {
				originalTime := metav1.NewTime(time.Now().Add(-1 * time.Hour))
				conditions = []metav1.Condition{
					{
						Type:               ConditionTypeReady,
						Status:             metav1.ConditionTrue,
						Reason:             ReasonReconcileSuccess,
						Message:            "Ready",
						LastTransitionTime: originalTime,
					},
				}

				SetCondition(&conditions, ConditionTypeReady, metav1.ConditionTrue, ReasonReconcileSuccess, "Ready")

				Expect(conditions).To(HaveLen(1))
				Expect(conditions[0].LastTransitionTime).To(Equal(originalTime))
			})
		})
	})

	Describe("GetCondition", func() {
		var conditions []metav1.Condition

		BeforeEach(func() {
			conditions = []metav1.Condition{
				{
					Type:    ConditionTypeReady,
					Status:  metav1.ConditionTrue,
					Reason:  ReasonReconcileSuccess,
					Message: "Ready",
				},
				{
					Type:    ConditionTypeConnected,
					Status:  metav1.ConditionFalse,
					Reason:  ReasonConnectionFailed,
					Message: "Connection failed",
				},
			}
		})

		Context("when condition exists", func() {
			It("should return the condition", func() {
				cond := GetCondition(conditions, ConditionTypeReady)

				Expect(cond).NotTo(BeNil())
				Expect(cond.Type).To(Equal(ConditionTypeReady))
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})

			It("should return correct condition by type", func() {
				cond := GetCondition(conditions, ConditionTypeConnected)

				Expect(cond).NotTo(BeNil())
				Expect(cond.Type).To(Equal(ConditionTypeConnected))
				Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Context("when condition does not exist", func() {
			It("should return nil", func() {
				cond := GetCondition(conditions, ConditionTypeHealthy)

				Expect(cond).To(BeNil())
			})
		})

		Context("when conditions list is empty", func() {
			It("should return nil", func() {
				cond := GetCondition([]metav1.Condition{}, ConditionTypeReady)

				Expect(cond).To(BeNil())
			})
		})

		Context("when conditions list is nil", func() {
			It("should return nil", func() {
				cond := GetCondition(nil, ConditionTypeReady)

				Expect(cond).To(BeNil())
			})
		})
	})

	Describe("IsConditionTrue", func() {
		Context("when condition is true", func() {
			It("should return true", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionTrue,
					},
				}

				Expect(IsConditionTrue(conditions, ConditionTypeReady)).To(BeTrue())
			})
		})

		Context("when condition is false", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionFalse,
					},
				}

				Expect(IsConditionTrue(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when condition is unknown", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionUnknown,
					},
				}

				Expect(IsConditionTrue(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when condition does not exist", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{}

				Expect(IsConditionTrue(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when conditions is nil", func() {
			It("should return false", func() {
				Expect(IsConditionTrue(nil, ConditionTypeReady)).To(BeFalse())
			})
		})
	})

	Describe("IsConditionFalse", func() {
		Context("when condition is false", func() {
			It("should return true", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionFalse,
					},
				}

				Expect(IsConditionFalse(conditions, ConditionTypeReady)).To(BeTrue())
			})
		})

		Context("when condition is true", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionTrue,
					},
				}

				Expect(IsConditionFalse(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when condition is unknown", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{
					{
						Type:   ConditionTypeReady,
						Status: metav1.ConditionUnknown,
					},
				}

				Expect(IsConditionFalse(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when condition does not exist", func() {
			It("should return false", func() {
				conditions := []metav1.Condition{}

				Expect(IsConditionFalse(conditions, ConditionTypeReady)).To(BeFalse())
			})
		})

		Context("when conditions is nil", func() {
			It("should return false", func() {
				Expect(IsConditionFalse(nil, ConditionTypeReady)).To(BeFalse())
			})
		})
	})

	Describe("RemoveCondition", func() {
		var conditions []metav1.Condition

		BeforeEach(func() {
			conditions = []metav1.Condition{
				{
					Type:    ConditionTypeReady,
					Status:  metav1.ConditionTrue,
					Reason:  ReasonReconcileSuccess,
					Message: "Ready",
				},
				{
					Type:    ConditionTypeConnected,
					Status:  metav1.ConditionTrue,
					Reason:  ReasonConnectionSuccess,
					Message: "Connected",
				},
				{
					Type:    ConditionTypeHealthy,
					Status:  metav1.ConditionTrue,
					Reason:  ReasonHealthCheckPassed,
					Message: "Healthy",
				},
			}
		})

		Context("when condition exists", func() {
			It("should remove the condition", func() {
				RemoveCondition(&conditions, ConditionTypeConnected)

				Expect(conditions).To(HaveLen(2))
				Expect(GetCondition(conditions, ConditionTypeConnected)).To(BeNil())
			})

			It("should preserve other conditions", func() {
				RemoveCondition(&conditions, ConditionTypeConnected)

				Expect(GetCondition(conditions, ConditionTypeReady)).NotTo(BeNil())
				Expect(GetCondition(conditions, ConditionTypeHealthy)).NotTo(BeNil())
			})

			It("should remove first condition", func() {
				RemoveCondition(&conditions, ConditionTypeReady)

				Expect(conditions).To(HaveLen(2))
				Expect(GetCondition(conditions, ConditionTypeReady)).To(BeNil())
			})

			It("should remove last condition", func() {
				RemoveCondition(&conditions, ConditionTypeHealthy)

				Expect(conditions).To(HaveLen(2))
				Expect(GetCondition(conditions, ConditionTypeHealthy)).To(BeNil())
			})
		})

		Context("when condition does not exist", func() {
			It("should not modify the list", func() {
				RemoveCondition(&conditions, ConditionTypeSynced)

				Expect(conditions).To(HaveLen(3))
			})
		})

		Context("when removing all conditions one by one", func() {
			It("should result in empty list", func() {
				RemoveCondition(&conditions, ConditionTypeReady)
				RemoveCondition(&conditions, ConditionTypeConnected)
				RemoveCondition(&conditions, ConditionTypeHealthy)

				Expect(conditions).To(BeEmpty())
			})
		})

		Context("when conditions list is empty", func() {
			It("should not panic", func() {
				emptyConditions := []metav1.Condition{}

				Expect(func() {
					RemoveCondition(&emptyConditions, ConditionTypeReady)
				}).NotTo(Panic())
			})
		})
	})

	Describe("Helper Functions", func() {
		var conditions []metav1.Condition

		BeforeEach(func() {
			conditions = []metav1.Condition{}
		})

		Describe("SetReadyCondition", func() {
			It("should set Ready condition", func() {
				SetReadyCondition(&conditions, metav1.ConditionTrue, ReasonReconcileSuccess, "Resource is ready")

				cond := GetCondition(conditions, ConditionTypeReady)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Describe("SetConnectedCondition", func() {
			It("should set Connected condition", func() {
				SetConnectedCondition(&conditions, metav1.ConditionTrue, ReasonConnectionSuccess, "Connected")

				cond := GetCondition(conditions, ConditionTypeConnected)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Describe("SetHealthyCondition", func() {
			It("should set Healthy condition", func() {
				SetHealthyCondition(&conditions, metav1.ConditionTrue, ReasonHealthCheckPassed, "Healthy")

				cond := GetCondition(conditions, ConditionTypeHealthy)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})

		Describe("SetSyncedCondition", func() {
			It("should set Synced condition", func() {
				SetSyncedCondition(&conditions, metav1.ConditionTrue, ReasonReconcileSuccess, "Synced")

				cond := GetCondition(conditions, ConditionTypeSynced)
				Expect(cond).NotTo(BeNil())
				Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			})
		})
	})

	Describe("Condition Constants", func() {
		It("should have expected condition type values", func() {
			Expect(ConditionTypeReady).To(Equal("Ready"))
			Expect(ConditionTypeConnected).To(Equal("Connected"))
			Expect(ConditionTypeHealthy).To(Equal("Healthy"))
			Expect(ConditionTypeSynced).To(Equal("Synced"))
			Expect(ConditionTypeProgressing).To(Equal("Progressing"))
			Expect(ConditionTypeDegraded).To(Equal("Degraded"))
		})

		It("should have expected reason values", func() {
			Expect(ReasonReconciling).To(Equal("Reconciling"))
			Expect(ReasonReconcileSuccess).To(Equal("ReconcileSuccess"))
			Expect(ReasonReconcileFailed).To(Equal("ReconcileFailed"))
			Expect(ReasonConnecting).To(Equal("Connecting"))
			Expect(ReasonConnectionSuccess).To(Equal("ConnectionSuccess"))
			Expect(ReasonConnectionFailed).To(Equal("ConnectionFailed"))
			Expect(ReasonCreating).To(Equal("Creating"))
			Expect(ReasonCreated).To(Equal("Created"))
			Expect(ReasonCreateFailed).To(Equal("CreateFailed"))
			Expect(ReasonDeleting).To(Equal("Deleting"))
			Expect(ReasonDeleted).To(Equal("Deleted"))
			Expect(ReasonDeleteFailed).To(Equal("DeleteFailed"))
			Expect(ReasonHealthCheckPassed).To(Equal("HealthCheckPassed"))
			Expect(ReasonHealthCheckFailed).To(Equal("HealthCheckFailed"))
		})
	})
})
