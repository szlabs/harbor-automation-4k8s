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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kstatus/status"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/legacy"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/model"
)

const (
	defaultCycle    = 5 * time.Minute
	defaultStatus   = "Unknown"
	unhealthyStatus = "UnHealthy"
	defaultComp     = "Harbor"
)

// HarborServerConfigurationReconciler reconciles a HarborServerConfiguration object
type HarborServerConfigurationReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Harbor *legacy.Client
}

// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=harborserverconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=harborserverconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile the HarborServerConfiguration
func (r *HarborServerConfigurationReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("harborserverconfiguration", req.NamespacedName)

	// Get the configuration first
	hsc := &goharborv1alpha1.HarborServerConfiguration{}
	if err := r.Client.Get(ctx, req.NamespacedName, hsc); err != nil {
		if apierr.IsNotFound(err) {
			// It could have been deleted after reconcile request coming in.
			log.Info("Harbor server configuration does not exist")
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get HarborServerConfiguraiton error: %w", err)
	}

	// Check if the server configuration is valid.
	// That is checking if the admin password secret object is valid.
	// Create harbor client
	err := r.createHarborClient(ctx, hsc)
	if err != nil {
		return ctrl.Result{}, nil
	}

	// Check if the configuration is being deleted
	if !hsc.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("Harbor server configuration is being deleted")
		return ctrl.Result{}, nil
	}

	// Check server health and construct status
	st, cerr := r.checkServerHealth()

	// Update status first for both success and failed checks
	hsc.Status = st
	if err := r.Client.Status().Update(ctx, hsc); err != nil {
		res := ctrl.Result{
			Requeue: true,
		}

		sec, wait := apierr.SuggestsClientDelay(err)
		if wait {
			res.RequeueAfter = time.Second * time.Duration(sec)
		}

		if apierr.IsConflict(err) {
			// if it's conflict, requeue
			r.Log.Error(err, "failed to update status")
			return res, nil
		}

		return res, fmt.Errorf("failed to update status with error: %w", err)
	}

	if cerr != nil {
		return ctrl.Result{}, cerr
	}

	// The health should be rechecked after a reasonable cycle
	return ctrl.Result{
		RequeueAfter: defaultCycle,
	}, nil
}

// SetupWithManager for HarborServerConfiguration reconcile controller
func (r *HarborServerConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&goharborv1alpha1.HarborServerConfiguration{}).
		Complete(r)
}

func (r *HarborServerConfigurationReconciler) checkServerHealth() (goharborv1alpha1.HarborServerConfigurationStatus, error) {
	overallStatus := goharborv1alpha1.HarborServerConfigurationStatus{
		Status:     defaultStatus,
		Conditions: make([]goharborv1alpha1.Condition, 0),
	}

	healthPayload, err := r.Harbor.CheckHealth()
	if err != nil {
		overallStatus.Conditions = append(overallStatus.Conditions, goharborv1alpha1.Condition{
			Type:    status.ConditionType(defaultComp),
			Status:  corev1.ConditionFalse,
			Message: "check health error",
			Reason:  err.Error(),
		})
		overallStatus.Status = unhealthyStatus
		return overallStatus, err
	}

	overallStatus.Status = healthPayload.Status
	for _, comp := range healthPayload.Components {
		cond := goharborv1alpha1.Condition{
			Type:   status.ConditionType(comp.Name),
			Status: corev1.ConditionTrue,
		}

		if len(comp.Error) > 0 {
			cond.Status = corev1.ConditionFalse
			cond.Reason = comp.Error
			cond.Message = "An error occurred"
		}

		overallStatus.Conditions = append(overallStatus.Conditions, cond)
	}

	return overallStatus, nil
}

func (r *HarborServerConfigurationReconciler) createHarborClient(ctx context.Context, hsc *goharborv1alpha1.HarborServerConfiguration) error {
	// contruct accessCreds from Secret
	cred, err := r.createAccessCredsFromSecret(ctx, hsc)
	if err != nil {
		return err
	}
	// put server config into client
	server := model.NewHarborServer(hsc.Spec.ServerURL, cred, hsc.Spec.InSecure)
	r.Harbor.WithContext(ctx).WithServer(server)

	return nil
}

func (r *HarborServerConfigurationReconciler) createAccessCredsFromSecret(ctx context.Context, hsc *goharborv1alpha1.HarborServerConfiguration) (*model.AccessCred, error) {
	accessSecret := &corev1.Secret{}
	secretNSedName := types.NamespacedName{
		Namespace: hsc.Spec.AccessCredential.Namespace,
		Name:      hsc.Spec.AccessCredential.AccessSecretRef,
	}

	if err := r.Client.Get(ctx, secretNSedName, accessSecret); err != nil {
		// No matter what errors (including not found) occurred, the server configuration is invalid
		return nil, fmt.Errorf("get access secret error: %w", err)
	}

	// convert secrets to AccessCred
	cred := &model.AccessCred{}
	if err := cred.FillIn(accessSecret); err != nil {
		return nil, fmt.Errorf("fill in secret error: %w", err)
	}

	return cred, nil
}
