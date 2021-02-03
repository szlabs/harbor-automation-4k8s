package hsc

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
)

// +kubebuilder:webhook:path=/validate-hsc,mutating=false,failurePolicy=fail,groups="goharbor.goharbor.io",resources=harborserverconfigurations,verbs=create;update,versions=v1alpha1,name=hsc.goharbor.io
type HSCValidator struct {
	Client  client.Client
	Log     logr.Logger
	decoder *admission.Decoder
}

var _ admission.Handler = (*HSCValidator)(nil)
var _ admission.DecoderInjector = (*HSCValidator)(nil)

func (h *HSCValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	hsc := &goharborv1alpha1.HarborServerConfiguration{}

	err := h.decoder.Decode(req, hsc)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	// Check for duplicate default configurations
	if hsc.Spec.Default {
		hscList := &goharborv1alpha1.HarborServerConfigurationList{}
		if err := h.Client.List(ctx, hscList); err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("failed to list harbor server configurations: %w", err))
		}
		for _, harborConf := range hscList.Items {
			if harborConf.Name != hsc.Name && harborConf.Spec.Default {
				return admission.ValidationResponse(false, fmt.Sprintf("%q can not be set as default, %q is the default harbor server configuration", hsc.Name, harborConf.Name))
			}
		}
	}
	return admission.Allowed("")
}

func (h *HSCValidator) InjectDecoder(decoder *admission.Decoder) error {
	h.decoder = decoder
	return nil
}

func (h *HSCValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&goharborv1alpha1.HarborServerConfiguration{}).Complete()
}
