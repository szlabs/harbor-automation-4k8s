// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-image-path,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,sideEffects=NoneOnDryRun,admissionReviewVersions=v1beta1,versions=v1,name=mimg.kb.io

// ImagePathRewriter implements webhook logic to mutate the image path of deploying pods
type ImagePathRewriter struct {
	Client  client.Client
	Log     logr.Logger
	decoder *admission.Decoder
}

// Handle the admission webhook for mutating the image path of deploying pods
func (ipr *ImagePathRewriter) Handle(ctx context.Context, req admission.Request) admission.Response {
	pod := &corev1.Pod{}

	err := ipr.decoder.Decode(req, pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Get namespace of pod
	podNS, err := ipr.getPodNamespace(ctx, req.Namespace)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get pod namespace object error: %w", err))
	}

	ipr.Log.V(4).Info("receive pod request", "pod", pod)

	// whether to rewrite image path is dependent on rules
	// the rules could be in assigned hsc or default hsc
	var hsc *goharborv1alpha1.HarborServerConfiguration
	if issuer, yes := podNS.Annotations[utils.AnnotationHarborServer]; yes {
		hsc, err = ipr.getHarborServerConfig(ctx, pod.Namespace, issuer)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	defaultHSC, err := ipr.lookupDefaultHarborServerConfig(ctx)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get default hsc object error: %w", err))
	}

	// there is no assigned or default hsc, skip
	if hsc == nil && defaultHSC == nil {
		return admission.Allowed("no change")
	}

	// merge rule in assigned and default hsc
	imageRules := mergeImageRule(hsc, defaultHSC)
	if imageRules == nil || len(imageRules) == 0 {
		return admission.Allowed("no change")
	}

	return ipr.rewriteContainers(req, imageRules, pod)
}

type rule struct {
	registryRegex string
	project       string
	serverURL     string
}

// merge rules between assigned hsc and default one
// assigned hsc has higher priority than hsc
func mergeImageRule(assignedHSC, defaultHSC *goharborv1alpha1.HarborServerConfiguration) []rule {
	var res []rule
	assignedRule := make(map[string]struct{})
	if assignedHSC != nil && assignedHSC.Spec.Rules != nil && len(assignedHSC.Spec.Rules) != 0 {
		for _, r := range assignedHSC.Spec.Rules {
			registryRegex := r[:strings.LastIndex(r, ",")+1]
			project := r[strings.LastIndex(r, ",")+1:]
			res = append(res, rule{
				registryRegex: registryRegex,
				project:       project,
				serverURL:     assignedHSC.Spec.ServerURL})
			assignedRule[r[:strings.LastIndex(r, ",")+1]] = struct{}{}
		}
	}
	if defaultHSC != nil && defaultHSC.Spec.Rules != nil && len(defaultHSC.Spec.Rules) != 0 {
		for _, r := range defaultHSC.Spec.Rules {
			registryRegex := r[:strings.LastIndex(r, ",")+1]
			// if there is conflict, skip the rule in default hsc
			if _, ok := assignedRule[registryRegex]; !ok {
				project := r[strings.LastIndex(r, ",")+1:]
				res = append(res, rule{
					registryRegex: registryRegex,
					project:       project,
					serverURL:     defaultHSC.Spec.ServerURL})
			}

		}
	}
	return res
}

func (ipr *ImagePathRewriter) rewriteContainers(req admission.Request, rules []rule, pod *corev1.Pod) admission.Response {
	for i, c := range pod.Spec.Containers {
		rewrittenImage, err := rewriteContainer(c.Image, rules)
		if err != nil {
			ipr.Log.Error(err, "invalid container image format", "image", c.Image)
			continue
		}
		if rewrittenImage != "" {
			rewrittenContainer := c.DeepCopy()
			rewrittenContainer.Image = rewrittenImage
			pod.Spec.Containers[i] = *rewrittenContainer
			ipr.Log.Info("rewrite container image", "original", c.Image, "rewrite", rewrittenImage)
		}
	}

	for i, c := range pod.Spec.InitContainers {
		rewrittenImage, err := rewriteContainer(c.Image, rules)
		if err != nil {
			ipr.Log.Error(err, "invalid container image format", "image", c.Image)
			continue
		}
		if rewrittenImage != "" {
			rewrittenContainer := c.DeepCopy()
			rewrittenContainer.Image = rewrittenImage
			pod.Spec.InitContainers[i] = *rewrittenContainer
			ipr.Log.Info("rewrite init image", "original", c.Image, "rewrite", rewrittenImage)
		}
	}

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func (ipr *ImagePathRewriter) lookupDefaultHarborServerConfig(ctx context.Context) (*goharborv1alpha1.HarborServerConfiguration, error) {
	hscList := &goharborv1alpha1.HarborServerConfigurationList{}
	if err := ipr.Client.List(ctx, hscList); err != nil {
		return nil, err
	}
	for _, hsc := range hscList.Items {
		if hsc.Spec.Default {
			return &hsc, nil
		}
	}
	return nil, nil
}

// A decoder will be automatically injected.
// InjectDecoder injects the decoder
func (ipr *ImagePathRewriter) InjectDecoder(d *admission.Decoder) error {
	ipr.decoder = d
	return nil
}

func (ipr *ImagePathRewriter) getPodNamespace(ctx context.Context, ns string) (*corev1.Namespace, error) {
	namespace := &corev1.Namespace{}

	nsName := types.NamespacedName{
		Namespace: "",
		Name:      ns,
	}
	if err := ipr.Client.Get(ctx, nsName, namespace); err != nil {
		return nil, fmt.Errorf("get namesapce error: %w", err)
	}

	return namespace, nil
}

func (ipr *ImagePathRewriter) getHarborServerConfig(ctx context.Context, ns string, issuer string) (*goharborv1alpha1.HarborServerConfiguration, error) {
	hsc := &goharborv1alpha1.HarborServerConfiguration{}
	nsName := types.NamespacedName{
		Name:      issuer,
		Namespace: ns,
	}

	if err := ipr.Client.Get(ctx, nsName, hsc); err != nil {
		return nil, err
	}

	return hsc, nil
}
