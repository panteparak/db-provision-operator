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
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AnnotationConfirmForceDelete is the annotation key for confirming force-delete cascade.
const AnnotationConfirmForceDelete = "dbops.dbprovision.io/confirm-force-delete"

// ComputeDeletionHash computes a short hash from sorted child resource names.
// The hash changes when children are added or removed, forcing re-confirmation.
func ComputeDeletionHash(children []string) string {
	sorted := make([]string, len(children))
	copy(sorted, children)
	sort.Strings(sorted)
	h := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return fmt.Sprintf("%x", h[:4]) // 8 hex chars
}

// GetConfirmForceDeleteAnnotation reads the confirm-force-delete annotation value.
func GetConfirmForceDeleteAnnotation(obj client.Object) string {
	if annotations := obj.GetAnnotations(); annotations != nil {
		return annotations[AnnotationConfirmForceDelete]
	}
	return ""
}

// IsForceDeleteConfirmed checks if the confirm annotation matches the hash
// computed from the current list of children.
func IsForceDeleteConfirmed(obj client.Object, children []string) bool {
	return GetConfirmForceDeleteAnnotation(obj) == ComputeDeletionHash(children)
}
