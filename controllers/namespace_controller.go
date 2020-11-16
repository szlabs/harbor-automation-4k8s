/*


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

package controllers

import (
	"context"
	"fmt"

	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
)

const (
	annotationIssuer  = "goharbor.io/secret-issuer"
	annotationAccount = "goharbor.io/service-account"
	defaultSa         = "default"
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=pullsecretbindings,verbs=get;list;watch;create;delete

func (r *NamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("namespace", req.NamespacedName)

	// Get the namespace object
	ns := &corev1.Namespace{}
	if err := r.Client.Get(ctx, req.NamespacedName, ns); err != nil {
		if apierr.IsNotFound(err) {
			// The resource may have be deleted after reconcile request coming in
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get namespace error: %w", err)
	}

	// Check if the ns is being deleted
	if !ns.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("namespace is being deleted", "name", ns.Name)
		return ctrl.Result{}, nil
	}

	// Get the binding list if existing
	bindings := &goharborv1alpha1.PullSecretBindingList{}
	if err := r.Client.List(ctx, bindings, &client.ListOptions{Namespace: req.Name}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list bindings error: %w", err)
	}

	// If auto-wire is set
	harborCfg, yes := ns.Annotations[annotationIssuer]
	if !yes {
		// If the annotation is removed and binding CRs are existing
		if len(bindings.Items) > 0 {
			for _, bd := range bindings.Items {
				// Remove all the existing bindings as issuer is removed
				if err := r.Client.Delete(ctx, &bd, &client.DeleteOptions{}); err != nil {
					// Retry next time
					return ctrl.Result{}, fmt.Errorf("remove binding %s error: %w", bd.Name, err)
				}
			}
		}

		// Match desired status, no issuers and then no bindings
		// Reconcile is completed
		return ctrl.Result{}, nil
	}

	// Pull secret issuer is set and then check if the required default binding is existing
	// Confirm the service account name
	sa := defaultSa
	if setSa, ok := ns.Annotations[annotationAccount]; ok {
		sa = setSa
	}

	// Find it
	for _, bd := range bindings.Items {
		if bd.Spec.HarborServerConfig == harborCfg && bd.Spec.ServiceAccount == sa {
			// Found it and reconcile is done
			return ctrl.Result{}, nil
		}
	}

	// Not existing, create one
	defaultBinding := r.getNewBindingCR(ns.Name, harborCfg, sa)
	if err := controllerutil.SetControllerReference(ns, defaultBinding, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("set crtl reference error: %w", err)
	}
	if err := r.Client.Create(ctx, defaultBinding, &client.CreateOptions{}); err != nil {
		return ctrl.Result{}, fmt.Errorf("create binding CR error: %w", err)
	}

	log.Info("create pull secret binding", "name", defaultBinding.Name)

	return ctrl.Result{}, nil
}

func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func (r *NamespaceReconciler) getNewBindingCR(ns string, harborCfg string, sa string) *goharborv1alpha1.PullSecretBinding {
	return &goharborv1alpha1.PullSecretBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RandomName("Binding"),
			Namespace: ns,
		},
		Spec: goharborv1alpha1.PullSecretBindingSpec{
			HarborServerConfig: harborCfg,
			ServiceAccount:     sa,
		},
	}
}
