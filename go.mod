module github.com/szlabs/harbor-automation-4k8s

go 1.13

require (
	github.com/containers/image/v5 v5.9.0
	github.com/go-logr/logr v0.1.0
	github.com/go-openapi/errors v0.19.2
	github.com/go-openapi/runtime v0.19.4
	github.com/go-openapi/strfmt v0.19.3
	github.com/go-openapi/swag v0.19.5
	github.com/go-openapi/validate v0.19.5
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	github.com/stretchr/testify v1.6.1
	github.com/umisama/go-regexpcache v0.0.0-20150417035358-2444a542492f
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kustomize/kstatus v0.0.2
)
