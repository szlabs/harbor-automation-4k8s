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

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// TODO: use same consts with namespace ctrl
	defaultSa = "default"
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

	flag, flagPresent := podNS.Annotations[utils.AnnotationRewriter]
	// if image path rewrite flag is not set or empty, skip
	if flag == "" || !flagPresent {
		return admission.Allowed("image rewriting disabled")
	}

	// If pod image path rewrite flag is set
	if flag == utils.ImageRewriteAuto || flag == utils.ImageRewriteRules {
		// fetch harbor-server from assigned hsc or from global hsc
		var hsc *goharborv1alpha1.HarborServerConfiguration
		var err error
		if issuer, yes := podNS.Annotations[utils.AnnotationHarborServer]; yes {
			hsc, err = ipr.getHarborServerConfig(ctx, pod.Namespace, issuer)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
		} else {
			hsc, err = ipr.lookupDefaultHarborServerConfig(ctx)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get default hsc object error: %w", err))
			}
		}

		// there is no assigned or default hsc, skip
		if hsc == nil {
			return admission.Errored(http.StatusInternalServerError,
				fmt.Errorf(`there is no hsc assigned or global default hsc for this namespace.
				but the image rewrite rule for this namespace is %s`, flag))
		}

		var imageRules []goharborv1alpha1.ImageRule
		if flag == utils.ImageRewriteAuto {
			sa := defaultSa
			if setSa, exists := podNS.Annotations[utils.AnnotationAccount]; exists {
				sa = setSa
			}

			ipr.Log.Info("get issuer and bound sa", "issuer", hsc.Name, "sa", sa)
			psb, err := ipr.getPullSecretBinding(ctx, pod.Namespace, hsc.Name, sa)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}

			ipr.Log.Info("get pullsecretbinding CR", "psb", psb)

			// project is stored inside namespace annotation
			pro, bound := podNS.Annotations[utils.AnnotationProject]
			if !bound {
				// it should be created in namespace_controller already
				return admission.Errored(http.StatusInternalServerError, fmt.Errorf("%s of binding %s is empty", utils.AnnotationProject, psb.Name))
			}
			imageRules = []goharborv1alpha1.ImageRule{
				{
					RegistryRegex: "*",
					HarborProject: pro,
				},
			}
			ipr.Log.V(4).Info("use harbor project %s in rule", pro)
		} else { // flag == utils.ImageRewriteRules
			ipr.Log.V(4).Info("use global hsc rule %v", hsc.Spec.Rules)
			imageRules = hsc.Spec.Rules
			if len(imageRules) == 0 {
				// if there is not rule in hsc, don't change and allow
				return admission.Allowed("no change since there is no rule in global hsc")
			}
		}

		return ipr.rewriteContainers(req, hsc.Spec.ServerURL, imageRules, pod)
	}

	return admission.Allowed("no change")
}

func (ipr *ImagePathRewriter) rewriteContainers(req admission.Request, serverURL string, rules []goharborv1alpha1.ImageRule, pod *corev1.Pod) admission.Response {
	for i, c := range pod.Spec.Containers {
		rewrittenImage, err := rewriteContainer(c.Image, serverURL, rules)
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
		rewrittenImage, err := rewriteContainer(c.Image, serverURL, rules)
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

func (ipr *ImagePathRewriter) getPullSecretBinding(ctx context.Context, ns, issuer, sa string) (*goharborv1alpha1.PullSecretBinding, error) {
	bindings := &goharborv1alpha1.PullSecretBindingList{}
	if err := ipr.Client.List(ctx, bindings, client.InNamespace(ns)); err != nil {
		return nil, fmt.Errorf("get bindings error: %w", err)
	}

	for _, bd := range bindings.Items {
		if bd.Spec.HarborServerConfig == issuer && bd.Spec.ServiceAccount == sa {
			return bd.DeepCopy(), nil
		}
	}

	return nil, fmt.Errorf("no binding with issuer=%s and sa=%s found", issuer, sa)
}
