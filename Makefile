# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
VERSION ?= 0.0.1

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# dbprovision.io/db-provision-operator-bundle:$VERSION and dbprovision.io/db-provision-operator-catalog:$VERSION.
IMAGE_TAG_BASE ?= dbprovision.io/db-provision-operator

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.42.0
# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run unit tests (fast, no envtest).
	go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-envtest
test-envtest: manifests generate setup-envtest ## Run envtest-based controller and validation tests.
	@echo "Running envtest-based controller tests..."
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./internal/controller/... -v -tags=envtest -timeout=10m \
		-coverprofile cover-envtest.out
	@echo "Running CRD validation tests..."
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./api/v1alpha1/... -v -tags=envtest -timeout=5m \
		-coverprofile cover-validation.out

.PHONY: test-envtest-precommit
test-envtest-precommit: setup-envtest ## Run CRD validation tests via envtest (for pre-commit hook).
	@KUBEBUILDER_ASSETS="$$($(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./api/v1alpha1/... -tags=envtest -timeout=5m

.PHONY: test-integration
test-integration: manifests generate setup-envtest ## Run integration tests with testcontainers-go and profiling.
	@mkdir -p test-reports
	$(eval ADAPTER_PATH := $(if $(INTEGRATION_TEST_DATABASE),$(call get-adapter-path,$(INTEGRATION_TEST_DATABASE)),./internal/adapter/...))
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./internal/controller/... $(ADAPTER_PATH) -v -tags=integration -timeout=15m \
		-coverprofile cover-integration.out
	@echo "Integration tests completed. Reports available in test-reports/"

# Map database name to adapter package path
define get-adapter-path
$(if $(filter postgresql postgres,$(1)),./internal/adapter/postgres/...,\
$(if $(filter mysql mariadb,$(1)),./internal/adapter/mysql/...,\
$(if $(filter cockroachdb,$(1)),./internal/adapter/cockroachdb/...,\
./internal/adapter/...)))
endef

.PHONY: test-integration-security
test-integration-security: manifests generate setup-envtest ## Run integration security tests only.
	@mkdir -p test-reports
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test ./internal/controller/... -v -tags=integration -timeout=15m \
		-run "Security" -coverprofile cover-integration-security.out

.PHONY: clean-test-reports
clean-test-reports: ## Clean test reports directory.
	rm -rf test-reports/

.PHONY: test-benchmark
test-benchmark: setup-envtest ## Run benchmarks for controllers.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test -bench=. -benchmem -run=^$$ ./internal/controller/... | tee benchmark-results.txt

.PHONY: test-templates
test-templates: ## Compare Helm and Kustomize template outputs for equivalence.
	@echo "Comparing Helm and Kustomize templates..."
	go test -v ./test/template/... -count=1

.PHONY: setup-precommit
setup-precommit: ## Install pre-commit and its dependencies
	@echo "Installing pre-commit dependencies..."
	@command -v pre-commit >/dev/null || { echo "Installing pre-commit..."; pip install pre-commit; }
	@command -v golangci-lint >/dev/null || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; }
	@command -v shellcheck >/dev/null || { echo "shellcheck not found. Install from: https://github.com/koalaman/shellcheck#installing"; }
	@command -v helm >/dev/null || { echo "helm not found. Install from: https://helm.sh/docs/intro/install/"; }
	@command -v docker >/dev/null || { echo "docker not found. Required for hadolint-docker hook."; }
	@echo "Setting up pre-commit hooks..."
	pre-commit install --install-hooks
	@echo "Done! Run 'pre-commit run --all-files' to verify."

##@ E2E Testing (k3d)

# E2E Testing with k3d
E2E_K3D_CLUSTER ?= dbprov-e2e
E2E_IMG ?= db-provision-operator:e2e

.PHONY: e2e-logs
e2e-logs: ## Show operator logs from E2E cluster
	kubectl logs -n db-provision-operator-system -l control-plane=controller-manager -f

E2E_DEBUG_LOG ?= e2e-debug-$(E2E_DATABASE).log

.PHONY: e2e-debug
e2e-debug: ## Show debug information for E2E cluster (pods, events, Docker Compose logs) - also saves to $(E2E_DEBUG_LOG)
	@{ \
		echo "=== E2E Debug Log for $(E2E_DATABASE) ===" ; \
		echo "=== Generated at $$(date -u '+%Y-%m-%d %H:%M:%S UTC') ===" ; \
		echo "" ; \
		echo "=== Nodes ===" ; \
		kubectl get nodes -o wide ; \
		echo "" ; \
		echo "=== Pods (all namespaces) ===" ; \
		kubectl get pods -A -o wide ; \
		echo "" ; \
		echo "=== Events (last 50) ===" ; \
		kubectl get events -A --sort-by='.lastTimestamp' | tail -50 ; \
		echo "" ; \
		echo "=== Operator Logs (last 100 lines) ===" ; \
		kubectl logs -n db-provision-operator-system -l control-plane=controller-manager --tail=100 2>&1 || true ; \
		echo "" ; \
		echo "=== Docker Compose Database Logs ===" ; \
		docker compose -f docker-compose.e2e.yml logs --tail=50 2>&1 || true ; \
		echo "" ; \
		echo "=== Docker Compose Service Status ===" ; \
		docker compose -f docker-compose.e2e.yml ps 2>&1 || true ; \
	} 2>&1 | tee $(E2E_DEBUG_LOG)
	@echo ""
	@echo "Debug log saved to: $(E2E_DEBUG_LOG)"

##@ E2E Local Testing (Docker Compose + k3d)
# These targets run databases in Docker Compose and operator in k3d
# Same micro-steps are used by CI for consistency

# Configuration (override via environment)
# Using higher ports to avoid conflicts with local database instances
E2E_POSTGRES_PORT ?= 15432
E2E_MYSQL_PORT ?= 13306
E2E_MARIADB_PORT ?= 13307
E2E_COCKROACHDB_PORT ?= 26257

# ─────────────────────────────────────────────────────────────────────────────
# MICRO-STEPS: Each can be called independently (by CI or locally)
# ─────────────────────────────────────────────────────────────────────────────

# Map E2E_DATABASE to compose service names (excluding init containers)
define get_compose_services
$(if $(filter postgresql,$(1)),postgres,$(if $(filter cockroachdb,$(1)),cockroachdb,$(1)))
endef

.PHONY: e2e-db-up
e2e-db-up: ## Start database(s) via Docker Compose. Use E2E_DATABASE to select specific engine (postgresql|mysql|mariadb|cockroachdb)
ifdef E2E_DATABASE
	@echo "Starting $(E2E_DATABASE) on port $(call get_e2e_port,$(E2E_DATABASE))..."
	E2E_POSTGRES_PORT=$(E2E_POSTGRES_PORT) E2E_MYSQL_PORT=$(E2E_MYSQL_PORT) E2E_MARIADB_PORT=$(E2E_MARIADB_PORT) E2E_COCKROACHDB_PORT=$(E2E_COCKROACHDB_PORT) \
		docker compose -f docker-compose.e2e.yml up -d --wait --wait-timeout 180 $(call get_compose_services,$(E2E_DATABASE))
  ifeq ($(E2E_DATABASE),cockroachdb)
	@echo "Running CockroachDB initialization..."
	E2E_COCKROACHDB_PORT=$(E2E_COCKROACHDB_PORT) docker compose -f docker-compose.e2e.yml up cockroachdb-init
  endif
else
	@echo "Starting all databases (use E2E_DATABASE=<engine> to select one)..."
	E2E_POSTGRES_PORT=$(E2E_POSTGRES_PORT) E2E_MYSQL_PORT=$(E2E_MYSQL_PORT) E2E_MARIADB_PORT=$(E2E_MARIADB_PORT) E2E_COCKROACHDB_PORT=$(E2E_COCKROACHDB_PORT) \
		docker compose -f docker-compose.e2e.yml up -d --wait --wait-timeout 180 postgres mysql mariadb cockroachdb
	@echo "Running CockroachDB initialization..."
	E2E_COCKROACHDB_PORT=$(E2E_COCKROACHDB_PORT) docker compose -f docker-compose.e2e.yml up cockroachdb-init
endif
	@echo "Database(s) ready"

.PHONY: e2e-db-down
e2e-db-down: ## Stop database(s) and remove volumes. Use E2E_DATABASE to select specific engine
ifdef E2E_DATABASE
	docker compose -f docker-compose.e2e.yml rm -fsv $(call get_compose_services,$(E2E_DATABASE))
else
	docker compose -f docker-compose.e2e.yml down -v
endif

.PHONY: e2e-cluster-create
e2e-cluster-create: ## Create k3d cluster (idempotent)
	@if k3d cluster list 2>/dev/null | grep -q "$(E2E_K3D_CLUSTER)"; then \
		echo "Cluster $(E2E_K3D_CLUSTER) already exists"; \
	else \
		k3d cluster create $(E2E_K3D_CLUSTER) \
			--agents 0 --wait --timeout 120s --no-lb --no-rollback \
			--k3s-arg "--disable=traefik@server:*" \
			--k3s-arg "--disable=servicelb@server:*" \
			--k3s-arg "--disable=metrics-server@server:*"; \
	fi
	kubectl wait --for=condition=Ready nodes --all --timeout=60s

.PHONY: e2e-cluster-delete
e2e-cluster-delete: ## Delete k3d cluster
	k3d cluster delete $(E2E_K3D_CLUSTER) || true

.PHONY: e2e-docker-build
e2e-docker-build: ## Build operator Docker image for E2E
	docker build -t $(E2E_IMG) .

.PHONY: e2e-image-load
e2e-image-load: ## Load operator image into k3d cluster
	k3d image import $(E2E_IMG) -c $(E2E_K3D_CLUSTER)

.PHONY: e2e-install-crds
e2e-install-crds: ## Install CRDs into cluster
	$(MAKE) install

.PHONY: e2e-deploy-operator
e2e-deploy-operator: ## Deploy operator to cluster
	$(MAKE) deploy IMG=$(E2E_IMG)
	kubectl wait --for=condition=Available \
		deployment/db-provision-operator-controller-manager \
		-n db-provision-operator-system --timeout=120s

# Helper to get namespace based on database type (postgresql->postgres, others unchanged)
define get_e2e_namespace
$(if $(filter postgresql,$(1)),postgres,$(1))
endef

# Helper to get port based on database type
define get_e2e_port
$(if $(filter postgresql,$(1)),$(E2E_POSTGRES_PORT),$(if $(filter mysql,$(1)),$(E2E_MYSQL_PORT),$(if $(filter mariadb,$(1)),$(E2E_MARIADB_PORT),$(if $(filter cockroachdb,$(1)),$(E2E_COCKROACHDB_PORT),5432))))
endef

.PHONY: e2e-create-db-instance
e2e-create-db-instance: ## Create DatabaseInstance CR pointing to Docker host (requires E2E_DATABASE)
	@echo "Creating DatabaseInstance for $(E2E_DATABASE)..."
	@kubectl apply -f test/e2e/fixtures/$(E2E_DATABASE)-local.yaml
	@echo "Waiting for DatabaseInstance to be ready..."
	@kubectl wait --for=jsonpath='{.status.phase}'=Ready \
		databaseinstance -n $(call get_e2e_namespace,$(E2E_DATABASE)) --all --timeout=60s

.PHONY: e2e-local-run-tests
e2e-local-run-tests: ## Run E2E tests for local setup (requires E2E_DATABASE)
	@echo "Running E2E tests for $(E2E_DATABASE)..."
	E2E_DATABASE_ENGINE=$(E2E_DATABASE) \
	E2E_DATABASE_HOST=127.0.0.1 \
	E2E_DATABASE_PORT=$(call get_e2e_port,$(E2E_DATABASE)) \
	E2E_INSTANCE_HOST=host.k3d.internal \
	E2E_INSTANCE_PORT=$(call get_e2e_port,$(E2E_DATABASE)) \
	go test ./test/e2e/... -v -tags=e2e -ginkgo.focus="$(E2E_DATABASE)" -ginkgo.v -timeout=10m

# ─────────────────────────────────────────────────────────────────────────────
# UNIFIED TARGETS: One command to run everything
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: e2e-local-setup
e2e-local-setup: ## Set up local E2E environment (DB + k3d + operator). Use E2E_DATABASE for single DB or omit for all
	$(MAKE) e2e-db-up $(if $(E2E_DATABASE),E2E_DATABASE=$(E2E_DATABASE),)
	$(MAKE) e2e-cluster-create
	$(MAKE) e2e-docker-build
	$(MAKE) e2e-image-load
	$(MAKE) e2e-install-crds
	$(MAKE) e2e-deploy-operator

.PHONY: e2e-local-postgresql
e2e-local-postgresql: ## Run PostgreSQL E2E tests (single DB setup + test)
	$(MAKE) e2e-local-setup E2E_DATABASE=postgresql
	$(MAKE) e2e-create-db-instance E2E_DATABASE=postgresql
	$(MAKE) e2e-local-run-tests E2E_DATABASE=postgresql

.PHONY: e2e-local-mysql
e2e-local-mysql: ## Run MySQL E2E tests (single DB setup + test)
	$(MAKE) e2e-local-setup E2E_DATABASE=mysql
	$(MAKE) e2e-create-db-instance E2E_DATABASE=mysql
	$(MAKE) e2e-local-run-tests E2E_DATABASE=mysql

.PHONY: e2e-local-mariadb
e2e-local-mariadb: ## Run MariaDB E2E tests (single DB setup + test)
	$(MAKE) e2e-local-setup E2E_DATABASE=mariadb
	$(MAKE) e2e-create-db-instance E2E_DATABASE=mariadb
	$(MAKE) e2e-local-run-tests E2E_DATABASE=mariadb

.PHONY: e2e-local-cockroachdb
e2e-local-cockroachdb: ## Run CockroachDB E2E tests (single DB setup + test)
	$(MAKE) e2e-local-setup E2E_DATABASE=cockroachdb
	$(MAKE) e2e-create-db-instance E2E_DATABASE=cockroachdb
	$(MAKE) e2e-local-run-tests E2E_DATABASE=cockroachdb

.PHONY: e2e-local-all
e2e-local-all: ## Run all local E2E tests (starts all DBs)
	$(MAKE) e2e-local-setup
	$(MAKE) e2e-create-db-instance E2E_DATABASE=postgresql
	$(MAKE) e2e-local-run-tests E2E_DATABASE=postgresql
	$(MAKE) e2e-create-db-instance E2E_DATABASE=mysql
	$(MAKE) e2e-local-run-tests E2E_DATABASE=mysql
	$(MAKE) e2e-create-db-instance E2E_DATABASE=mariadb
	$(MAKE) e2e-local-run-tests E2E_DATABASE=mariadb
	$(MAKE) e2e-create-db-instance E2E_DATABASE=cockroachdb
	$(MAKE) e2e-local-run-tests E2E_DATABASE=cockroachdb

# ─────────────────────────────────────────────────────────────────────────────
# MULTI-OPERATOR E2E TESTING
# ─────────────────────────────────────────────────────────────────────────────

.PHONY: e2e-deploy-second-operator
e2e-deploy-second-operator: ## Deploy a second operator instance (instance-id=isolated) for multi-operator E2E testing
	$(HELM) install db-provision-operator-isolated $(CHART_DIR) \
		--namespace db-provision-operator-isolated \
		--create-namespace \
		--set image.repository=db-provision-operator \
		--set image.tag=e2e \
		--set image.pullPolicy=IfNotPresent \
		--set instanceId=isolated \
		--set leaderElect=true \
		--set crds.install=false \
		--wait --timeout 2m

.PHONY: e2e-undeploy-second-operator
e2e-undeploy-second-operator: ## Remove the second operator instance
	-$(HELM) uninstall db-provision-operator-isolated --namespace db-provision-operator-isolated
	-kubectl delete namespace db-provision-operator-isolated --ignore-not-found

.PHONY: e2e-run-multi-operator-tests
e2e-run-multi-operator-tests: ## Run multi-operator E2E tests (PostgreSQL only)
	@echo "Running multi-operator E2E tests..."
	E2E_DATABASE_ENGINE=postgresql \
	E2E_DATABASE_HOST=127.0.0.1 \
	E2E_DATABASE_PORT=$(E2E_POSTGRES_PORT) \
	E2E_INSTANCE_HOST=host.k3d.internal \
	E2E_INSTANCE_PORT=$(E2E_POSTGRES_PORT) \
	go test ./test/e2e/... -v -tags=e2e \
		-ginkgo.label-filter="multi-operator" -ginkgo.v -timeout=5m

.PHONY: e2e-local-cleanup
e2e-local-cleanup: ## Clean up local E2E environment. Use E2E_DATABASE to cleanup specific DB only
	$(MAKE) e2e-cluster-delete
	$(MAKE) e2e-db-down $(if $(E2E_DATABASE),E2E_DATABASE=$(E2E_DATABASE),)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Assets

.PHONY: copy-dashboards
copy-dashboards: ## Copy dashboards to Helm and Kustomize directories (for local development)
	@mkdir -p charts/db-provision-operator/dashboards
	@mkdir -p config/grafana/dashboards
	@cp dashboards/*.json charts/db-provision-operator/dashboards/
	@cp dashboards/*.json config/grafana/dashboards/
	@echo "Dashboards copied successfully"

.PHONY: clean-dashboards
clean-dashboards: ## Remove copied dashboard files
	@rm -rf charts/db-provision-operator/dashboards
	@rm -rf config/grafana/dashboards
	@echo "Dashboard copies removed"

##@ Documentation

.PHONY: docs-deps
docs-deps: ## Install documentation dependencies
	pip install -r docs/requirements.txt

.PHONY: docs-serve
docs-serve: docs-deps ## Serve documentation locally
	mkdocs serve --dev-addr=0.0.0.0:8000

.PHONY: docs-build
docs-build: docs-deps ## Build documentation
	mkdocs build --strict

.PHONY: docs-deploy
docs-deploy: docs-deps ## Deploy docs to gh-pages (manual)
	mkdocs gh-deploy --force

CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs
CRD_REF_DOCS_VERSION ?= v0.3.0

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	$(call go-install-tool,$(CRD_REF_DOCS),github.com/elastic/crd-ref-docs,$(CRD_REF_DOCS_VERSION))

.PHONY: generate-api-docs
generate-api-docs: crd-ref-docs ## Generate API reference documentation from CRD types.
	$(CRD_REF_DOCS) \
		--source-path=./api/v1alpha1 \
		--config=./docs/api/config.yaml \
		--output-path=./docs/api/reference.md \
		--renderer=markdown
	@echo "API documentation generated at docs/api/reference.md"

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name db-provision-operator-builder
	$(CONTAINER_TOOL) buildx use db-provision-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm db-provision-operator-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Helm

HELM ?= helm
CHART_DIR ?= charts/db-provision-operator

.PHONY: helm-lint
helm-lint: ## Lint Helm chart
	$(HELM) lint $(CHART_DIR)

.PHONY: helm-template
helm-template: ## Render Helm chart templates locally
	$(HELM) template db-provision-operator $(CHART_DIR) --namespace db-provision-operator-system

.PHONY: helm-package
helm-package: ## Package Helm chart
	mkdir -p dist
	$(HELM) package $(CHART_DIR) -d dist/

.PHONY: helm-install
helm-install: ## Install Helm chart to current cluster
	$(HELM) install db-provision-operator $(CHART_DIR) \
		--namespace db-provision-operator-system \
		--create-namespace \
		--set image.repository=$(IMAGE_TAG_BASE) \
		--set image.tag=$(VERSION)

.PHONY: helm-upgrade
helm-upgrade: ## Upgrade Helm release
	$(HELM) upgrade db-provision-operator $(CHART_DIR) \
		--namespace db-provision-operator-system \
		--set image.repository=$(IMAGE_TAG_BASE) \
		--set image.tag=$(VERSION)

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall Helm release
	$(HELM) uninstall db-provision-operator --namespace db-provision-operator-system

.PHONY: helm-update-crds
helm-update-crds: manifests ## Copy CRDs to Helm chart
	cp config/crd/bases/*.yaml $(CHART_DIR)/crds/

##@ Release

.PHONY: release-manifests
release-manifests: manifests kustomize ## Generate release manifests
	mkdir -p dist
	$(KUSTOMIZE) build config/crd > dist/crds.yaml
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

.PHONY: release
release: release-manifests helm-package ## Build all release artifacts
	@echo "Release artifacts generated in dist/"
	@ls -la dist/

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.18.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = $(LOCALBIN)/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.55.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool $(CONTAINER_TOOL) --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)
