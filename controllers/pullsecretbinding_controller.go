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

	apierr "k8s.io/apimachinery/pkg/api/errors"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
)

// PullSecretBindingReconciler reconciles a PullSecretBinding object
type PullSecretBindingReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=pullsecretbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=pullsecretbindings/status,verbs=get;update;patch

func (r *PullSecretBindingReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("pullsecretbinding", req.NamespacedName)

	// Get the namespace object
	bd := &goharborv1alpha1.PullSecretBinding{}
	if err := r.Client.Get(ctx, req.NamespacedName, bd); err != nil {
		if apierr.IsNotFound(err) {
			// The resource may have be deleted after reconcile request coming in
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get binding CR error: %w", err)
	}

	// Check if the binding is being deleted
	if !bd.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("pull secret binding is being deleted", "name", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// TODO:
	log.Info("========TODO", "name", bd)

	return ctrl.Result{}, nil
}

func (r *PullSecretBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&goharborv1alpha1.PullSecretBinding{}).
		Complete(r)
}
