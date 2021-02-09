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

	"github.com/go-logr/logr"
	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	// TODO: use same consts with namespace ctrl
	annotationIssuer  = "goharbor.io/secret-issuer"
	annotationAccount = "goharbor.io/service-account"
	defaultSa         = "default"
	annotationProject = "goharbor.io/project"

	annotationRewriter   = "goharbor.io/image-rewrite"
	imageRewriteAuto     = "auto"
	imageRewriteDisabled = "disabled"
)

// +kubebuilder:webhook:path=/mutate-image-path,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=mimg.kb.io

// ImagePathRewriter implements webhook logic to mutate the image path of deploying pods
type ImagePathRewriter struct {
	Client  client.Client
	Log     logr.Logger
	decoder *admission.Decoder
}

// Handle the admission webhook fro mutating the image path of deploying pods
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

	flag, flagPresent := podNS.Annotations[annotationRewriter]
	if flag == imageRewriteDisabled {
		return admission.Allowed("image rewriting disabled")
	}
	// If the rewrite annotation is not set, fallback to default hsc handling
	if !flagPresent {
		defaultHSC, err := ipr.lookupDefaultHarborServerConfig(ctx)
		if err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("get default hsc object error: %w", err))
		}
		if defaultHSC == nil {
			return admission.Allowed("no default harbor server configuration")
		}

		return ipr.rewriteContainers(req, defaultHSC.Spec.ServerURL, defaultHSC.Spec.Rules, pod)
	}
	// If pod image path rewrite flag is set
	if flag == imageRewriteAuto {
		// Whether related issuer (HarborServerConfiguration) is set or not
		if issuer, yes := podNS.Annotations[annotationIssuer]; yes {
			hsc, err := ipr.getHarborServerConfig(ctx, pod.Namespace, issuer)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}
			sa := defaultSa
			if setSa, exists := podNS.Annotations[annotationAccount]; exists {
				sa = setSa
			}

			ipr.Log.Info("get issuer and bound sa", "issuer", hsc.Name, "sa", sa)
			psb, err := ipr.getPullSecretBinding(ctx, pod.Namespace, hsc.Name, sa)
			if err != nil {
				return admission.Errored(http.StatusInternalServerError, err)
			}

			ipr.Log.Info("get pullsecretbinding CR", "psb", psb)

			pro, bound := psb.Annotations[annotationProject]
			if !bound {
				return admission.Errored(http.StatusInternalServerError, fmt.Errorf("%s of binding %s is empty", annotationProject, psb.Name))
			}
			imageRules := []goharborv1alpha1.ImageRule{
				{
					Registry:      BareRegistry,
					HarborProject: pro,
				},
			}
			return ipr.rewriteContainers(req, hsc.Spec.ServerURL, imageRules, pod)
		}
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
