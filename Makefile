
# Image URL to use all building/pushing image targets
IMG ?= goharbor/harbor-automation-4k8s:dev
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests dev-certificate
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

delete: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests cert-manager
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

cert-manager:
	kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v1.1.0/cert-manager.yaml
	kubectl rollout status deployment cert-manager-webhook -n cert-manager
	kubectl rollout status deployment cert-manager -n cert-manager
	kubectl rollout status deployment cert-manager-cainjector -n cert-manager

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# ---------
# For local development
TMPDIR ?= /tmp/
export TMPDIR

.PHONY: dev-certificate
dev-certificate:
	$(RM) -r "$(TMPDIR)k8s-webhook-server/serving-certs"
	mkdir -p "$(TMPDIR)k8s-webhook-server/serving-certs"
	openssl req \
		-new \
		-newkey rsa:4096 \
		-days 365 \
		-nodes \
		-x509 \
		-subj "/C=FR/O=Dev/OU=$(shell whoami)/CN=example.com" \
		-keyout "$(TMPDIR)k8s-webhook-server/serving-certs/tls.key" \
		-out "$(TMPDIR)k8s-webhook-server/serving-certs/tls.crt"

# ---------
# GENERATE HARBOR CLIENT SDK FROM THE SWAGGER FILE
GOCMD=$(shell which go)
DOCKERCMD=$(shell which docker)
SWAGGER := $(DOCKERCMD) run --rm -it -v $(HOME):$(HOME) -w $(shell pwd) quay.io/goswagger/swagger
HARBOR_SPEC := ./assets/api_swagger.yml
SDK_DIR := ./pkg/sdk/harbor
HARBOR_SPEC_V2 := ./assets/api_swagger_v2.yml
SDK_DIR_V2 := ./pkg/sdk/harbor_v2

swag_validate:
	@$(SWAGGER) validate $(HARBOR_SPEC)

swag_harbor_sdk: swag_validate
	@$(SWAGGER) generate client --spec=$(HARBOR_SPEC) --target=$(SDK_DIR) -A harbor --principal=basic

swag_validate_v2:
	@$(SWAGGER) validate $(HARBOR_SPEC_V2)

swag_harbor_sdk_v2: swag_validate_v2
	@$(SWAGGER) generate client --spec=$(HARBOR_SPEC_V2) --target=$(SDK_DIR_V2) -A harbor --principal=basic

