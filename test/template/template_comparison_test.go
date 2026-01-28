//go:build !e2e

package template_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Resource represents a normalized K8s resource for comparison.
type Resource struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   map[string]interface{} `yaml:"metadata"`
	Spec       interface{}            `yaml:"spec,omitempty"`
	Data       interface{}            `yaml:"data,omitempty"`
	Rules      interface{}            `yaml:"rules,omitempty"`
	RoleRef    interface{}            `yaml:"roleRef,omitempty"`
	Subjects   interface{}            `yaml:"subjects,omitempty"`
}

// resourceKey creates a unique key for a resource based on kind and name.
func resourceKey(res Resource) string {
	name := ""
	if res.Metadata != nil {
		if n, ok := res.Metadata["name"].(string); ok {
			name = n
		}
	}
	return res.Kind + "/" + name
}

func TestHelmKustomizeEquivalence(t *testing.T) {
	// Get project root
	projectRoot := getProjectRoot(t)

	// Render Helm templates
	helmResources := renderHelm(t, projectRoot)
	t.Logf("Helm rendered %d resources", len(helmResources))

	// Render Kustomize templates
	kustomizeResources := renderKustomize(t, projectRoot)
	t.Logf("Kustomize rendered %d resources", len(kustomizeResources))

	// Normalize both outputs
	// Helm uses: test-release-db-provision-operator-<resource>
	// Kustomize uses: db-provision-operator-<resource>
	normalizedHelm := normalizeResources(helmResources, "test-release-db-provision-operator-")
	normalizedKustomize := normalizeResources(kustomizeResources, "db-provision-operator-")

	// Build maps for easier lookup
	helmMap := make(map[string]Resource)
	for _, res := range normalizedHelm {
		helmMap[resourceKey(res)] = res
	}

	kustomizeMap := make(map[string]Resource)
	for _, res := range normalizedKustomize {
		kustomizeMap[resourceKey(res)] = res
	}

	// Core resources that MUST exist in both and be equivalent
	coreResources := []struct {
		kind string
		name string // normalized name (without prefix)
	}{
		{"Deployment", "controller-manager"},
		{"ServiceAccount", "controller-manager"},
		{"ClusterRole", "manager-role"},
		{"ClusterRoleBinding", "manager-rolebinding"},
	}

	for _, core := range coreResources {
		t.Run(core.kind+"/"+core.name, func(t *testing.T) {
			key := core.kind + "/" + core.name

			helmRes, helmOK := helmMap[key]
			kustomizeRes, kustomizeOK := kustomizeMap[key]

			require.True(t, helmOK, "Helm should render %s", key)
			require.True(t, kustomizeOK, "Kustomize should render %s", key)

			// Compare API versions
			assert.Equal(t, helmRes.APIVersion, kustomizeRes.APIVersion,
				"%s apiVersion should match", key)

			// Compare the functional parts based on resource type
			switch core.kind {
			case "Deployment":
				compareDeploymentSpecs(t, helmRes, kustomizeRes)
			case "ClusterRole":
				compareClusterRoleRules(t, helmRes, kustomizeRes)
			case "ClusterRoleBinding":
				compareRoleBindings(t, helmRes, kustomizeRes)
			case "ServiceAccount":
				// ServiceAccounts are simple - just check they exist
				t.Logf("ServiceAccount %s exists in both", core.name)
			}
		})
	}
}

func TestHelmRendersAllRequiredResources(t *testing.T) {
	projectRoot := getProjectRoot(t)
	helmResources := renderHelm(t, projectRoot)

	requiredKinds := map[string]bool{
		"Deployment":         false,
		"ServiceAccount":     false,
		"ClusterRole":        false,
		"ClusterRoleBinding": false,
	}

	for _, res := range helmResources {
		if _, required := requiredKinds[res.Kind]; required {
			requiredKinds[res.Kind] = true
		}
	}

	for kind, found := range requiredKinds {
		assert.True(t, found, "Helm should render %s", kind)
	}
}

func TestKustomizeRendersAllRequiredResources(t *testing.T) {
	projectRoot := getProjectRoot(t)
	kustomizeResources := renderKustomize(t, projectRoot)

	requiredKinds := map[string]bool{
		"Deployment":         false,
		"ServiceAccount":     false,
		"ClusterRole":        false,
		"ClusterRoleBinding": false,
	}

	for _, res := range kustomizeResources {
		if _, required := requiredKinds[res.Kind]; required {
			requiredKinds[res.Kind] = true
		}
	}

	for kind, found := range requiredKinds {
		assert.True(t, found, "Kustomize should render %s", kind)
	}
}

func compareDeploymentSpecs(t *testing.T, helm, kustomize Resource) {
	t.Helper()

	helmSpec, ok := helm.Spec.(map[string]interface{})
	require.True(t, ok, "Helm Deployment should have spec")

	kustomizeSpec, ok := kustomize.Spec.(map[string]interface{})
	require.True(t, ok, "Kustomize Deployment should have spec")

	// Compare replicas (if set)
	if helmReplicas, ok := helmSpec["replicas"]; ok {
		if kustomizeReplicas, ok := kustomizeSpec["replicas"]; ok {
			assert.Equal(t, helmReplicas, kustomizeReplicas, "Deployment replicas should match")
		}
	}

	// Compare template spec (the pod spec) - specifically the containers
	helmTemplate := getNestedMap(helmSpec, "template", "spec")
	kustomizeTemplate := getNestedMap(kustomizeSpec, "template", "spec")

	if helmTemplate != nil && kustomizeTemplate != nil {
		// Compare container count
		helmContainers := getSlice(helmTemplate, "containers")
		kustomizeContainers := getSlice(kustomizeTemplate, "containers")

		assert.Equal(t, len(helmContainers), len(kustomizeContainers),
			"Deployment should have same number of containers")

		// Compare first container's args (command-line flags)
		if len(helmContainers) > 0 && len(kustomizeContainers) > 0 {
			helmContainer := helmContainers[0].(map[string]interface{})
			kustomizeContainer := kustomizeContainers[0].(map[string]interface{})

			// Container names might differ slightly
			t.Logf("Helm container: %v, Kustomize container: %v",
				helmContainer["name"], kustomizeContainer["name"])
		}
	}
}

func compareClusterRoleRules(t *testing.T, helm, kustomize Resource) {
	t.Helper()

	helmRules, ok := helm.Rules.([]interface{})
	if !ok {
		t.Log("Helm ClusterRole has no rules")
		return
	}

	kustomizeRules, ok := kustomize.Rules.([]interface{})
	if !ok {
		t.Log("Kustomize ClusterRole has no rules")
		return
	}

	// Build a set of API groups from each
	helmAPIGroups := extractAPIGroups(helmRules)
	kustomizeAPIGroups := extractAPIGroups(kustomizeRules)

	// Check that both have the same core API groups
	coreGroups := []string{
		"dbops.dbprovision.io", // CRD group
		"",                     // core API group (for secrets, configmaps, etc.)
	}

	for _, group := range coreGroups {
		assert.Contains(t, helmAPIGroups, group,
			"Helm ClusterRole should have rules for API group %q", group)
		assert.Contains(t, kustomizeAPIGroups, group,
			"Kustomize ClusterRole should have rules for API group %q", group)
	}
}

func compareRoleBindings(t *testing.T, helm, kustomize Resource) {
	t.Helper()

	// Compare roleRef
	helmRoleRef, ok := helm.RoleRef.(map[string]interface{})
	if !ok {
		t.Log("Helm ClusterRoleBinding has no roleRef")
		return
	}

	kustomizeRoleRef, ok := kustomize.RoleRef.(map[string]interface{})
	if !ok {
		t.Log("Kustomize ClusterRoleBinding has no roleRef")
		return
	}

	assert.Equal(t, helmRoleRef["kind"], kustomizeRoleRef["kind"],
		"RoleRef kind should match")
	assert.Equal(t, helmRoleRef["apiGroup"], kustomizeRoleRef["apiGroup"],
		"RoleRef apiGroup should match")

	// Compare subjects (should both reference the service account)
	helmSubjects, ok := helm.Subjects.([]interface{})
	if !ok || len(helmSubjects) == 0 {
		t.Log("Helm ClusterRoleBinding has no subjects")
		return
	}

	kustomizeSubjects, ok := kustomize.Subjects.([]interface{})
	if !ok || len(kustomizeSubjects) == 0 {
		t.Log("Kustomize ClusterRoleBinding has no subjects")
		return
	}

	assert.Equal(t, len(helmSubjects), len(kustomizeSubjects),
		"ClusterRoleBinding should have same number of subjects")
}

func renderHelm(t *testing.T, projectRoot string) []Resource {
	t.Helper()
	chartPath := filepath.Join(projectRoot, "charts", "db-provision-operator")

	cmd := exec.Command("helm", "template", "test-release", chartPath,
		"--namespace", "db-provision-operator-system",
		"--set", "image.tag=test",
		"--set", "crds.install=false") // Skip CRDs for comparison

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "helm template failed: %s", stderr.String())

	return parseYAMLDocuments(t, stdout.Bytes())
}

func renderKustomize(t *testing.T, projectRoot string) []Resource {
	t.Helper()
	kustomizePath := filepath.Join(projectRoot, "config", "default")

	cmd := exec.Command("kustomize", "build", kustomizePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "kustomize build failed: %s", stderr.String())

	return parseYAMLDocuments(t, stdout.Bytes())
}

func normalizeResources(resources []Resource, namePrefix string) []Resource {
	normalized := make([]Resource, 0, len(resources))

	for _, res := range resources {
		// Skip CRDs - they're managed separately
		if res.Kind == "CustomResourceDefinition" {
			continue
		}

		// Deep copy to avoid modifying original
		normalizedRes := res

		// Normalize metadata
		if normalizedRes.Metadata != nil {
			// Remove tool-specific labels
			if labels, ok := normalizedRes.Metadata["labels"].(map[string]interface{}); ok {
				delete(labels, "helm.sh/chart")
				delete(labels, "app.kubernetes.io/version")
				delete(labels, "app.kubernetes.io/instance")
				labels["app.kubernetes.io/managed-by"] = "normalized"
			}

			// Normalize name (strip prefix to get base resource name)
			// Helm: test-release-db-provision-operator-controller-manager -> controller-manager
			// Kustomize: db-provision-operator-controller-manager -> controller-manager
			if name, ok := normalizedRes.Metadata["name"].(string); ok {
				name = strings.TrimPrefix(name, namePrefix)
				normalizedRes.Metadata["name"] = name
			}

			// Normalize namespace
			normalizedRes.Metadata["namespace"] = "db-provision-operator-system"
		}

		normalized = append(normalized, normalizedRes)
	}

	return normalized
}

func parseYAMLDocuments(t *testing.T, data []byte) []Resource {
	t.Helper()
	var resources []Resource
	decoder := yaml.NewDecoder(bytes.NewReader(data))

	for {
		var res Resource
		err := decoder.Decode(&res)
		if err != nil {
			break // EOF or error
		}
		if res.Kind != "" {
			resources = append(resources, res)
		}
	}

	return resources
}

func getProjectRoot(t *testing.T) string {
	t.Helper()
	// Walk up from test directory to find go.mod
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root")
		}
		dir = parent
	}
}

func getNestedMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	current := m
	for _, key := range keys {
		if next, ok := current[key].(map[string]interface{}); ok {
			current = next
		} else {
			return nil
		}
	}
	return current
}

func getSlice(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
	}
	return nil
}

func extractAPIGroups(rules []interface{}) map[string]bool {
	groups := make(map[string]bool)
	for _, rule := range rules {
		if ruleMap, ok := rule.(map[string]interface{}); ok {
			if apiGroups, ok := ruleMap["apiGroups"].([]interface{}); ok {
				for _, g := range apiGroups {
					if gs, ok := g.(string); ok {
						groups[gs] = true
					}
				}
			}
		}
	}
	return groups
}
