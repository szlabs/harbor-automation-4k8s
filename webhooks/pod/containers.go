package pod

import (
	"fmt"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
)

const BareRegistry = "docker.io"

// registryFromImageRef returns the registry (and port, if set) from the image reference,
// otherwise returns the default bare registry, "docker.io".
func registryFromImageRef(imageReference string) (registry string, err error) {
	ref, err := reference.ParseDockerRef(imageReference)
	if err != nil {
		return "", err
	}
	return reference.Domain(ref), nil
}

// replaceRegistryInImageRef returns the the image reference with the registry replaced.
func replaceRegistryInImageRef(imageReference, replacementRegistry string) (imageRef string, err error) {
	named, err := reference.ParseDockerRef(imageReference)
	if err != nil {
		return "", err
	}
	return strings.Replace(named.String(), reference.Domain(named), replacementRegistry, 1), nil
}

// rewriteContainer replaces any registries matching the image rules with the given serverURL
func rewriteContainer(imageReference, serverURL string, rules []goharborv1alpha1.ImageRule) (imageRef string, err error) {
	registry, err := registryFromImageRef(imageReference)
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		if registry == rule.Registry {
			rewritten := fmt.Sprintf("%s/%s", serverURL, rule.HarborProject)
			return replaceRegistryInImageRef(imageReference, rewritten)
		}
	}
	return "", nil
}
