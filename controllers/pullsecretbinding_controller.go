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
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
	"github.com/szlabs/harbor-automation-4k8s/pkg/registry/secret"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/legacy"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/model"
	v2 "github.com/szlabs/harbor-automation-4k8s/pkg/rest/v2"
	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"
)

const (
	defaultOwner = "harbor-automation-4k8s"
	regSecType   = "kubernetes.io/dockerconfigjson"
	datakey      = ".dockerconfigjson"
	finalizerID  = "psb.finalizers.resource.goharbor.io"
)

// PullSecretBindingReconciler reconciles a PullSecretBinding object
type PullSecretBindingReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	HarborV2 *v2.Client
	Harbor   *legacy.Client
}

// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=pullsecretbindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=pullsecretbindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=goharbor.goharbor.io,resources=harborserverconfigurations,verbs=get
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;create;update
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;update;patch

func (r *PullSecretBindingReconciler) Reconcile(req ctrl.Request) (res ctrl.Result, ferr error) {
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

	// Check binding resources
	server, sa, res, err := r.checkBindingRes(ctx, bd)
	if err != nil {
		return res, err
	} else {
		if server == nil || sa == nil {
			return res, err
		}
	}

	// Talk to this server
	r.HarborV2.WithServer(server).WithContext(ctx)
	r.Harbor.WithServer(server).WithContext(ctx)

	// Check if the binding is being deleted
	if bd.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(bd.ObjectMeta.Finalizers, finalizerID) {
			// Append finalizer
			bd.ObjectMeta.Finalizers = append(bd.ObjectMeta.Finalizers, finalizerID)
			if err := r.update(ctx, bd); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if utils.ContainsString(bd.ObjectMeta.Finalizers, finalizerID) {
			// Execute and remove our finalizer from the finalizer list
			if err := r.deleteExternalResources(bd); err != nil {
				return ctrl.Result{}, err
			}

			bd.ObjectMeta.Finalizers = utils.RemoveString(bd.ObjectMeta.Finalizers, finalizerID)
			if err := r.Client.Update(ctx, bd, &client.UpdateOptions{}); err != nil {
				return ctrl.Result{}, err
			}
		}

		log.Info("pull secret binding is being deleted")
		return ctrl.Result{}, nil
	}

	defer func() {
		if ferr != nil && bd.Status.Status != "error" {
			bd.Status.Status = "error"
			if bd.Status.Conditions == nil {
				bd.Status.Conditions = make([]goharborv1alpha1.Condition, 0)
			}
			if err := r.Status().Update(ctx, bd, &client.UpdateOptions{}); err != nil {
				log.Error(err, "defer update status error", "cause", err)
			}
		}
	}()

	projID, robotID := parseIntID(bd.Spec.ProjectID), parseIntID(bd.Spec.RobotID)

	// Bind robot to service account
	// TODO: may cause dirty robots at the harbor project side
	// TODO: check secret binding by get secret and service account
	_, ok := bd.Annotations[utils.AnnotationRobotSecretRef]
	if !ok {
		// Need to create a new one as we only have one time to get the robot token
		robot, err := r.Harbor.GetRobotAccount(projID, robotID)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create robot account error: %w", err)
		}

		// Make registry secret
		regsec, err := r.createRegSec(ctx, bd.Namespace, server.ServerURL, robot, bd)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create registry secret error: %w", err)
		}
		// Add secret to service account
		if sa.ImagePullSecrets == nil {
			sa.ImagePullSecrets = make([]corev1.LocalObjectReference, 0)
		}
		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{
			Name: regsec.Name,
		})

		// Update
		if err := r.Client.Update(ctx, sa, &client.UpdateOptions{}); err != nil {
			return ctrl.Result{}, fmt.Errorf("update error: %w", err)
		}

		// Update binding
		if err := controllerutil.SetControllerReference(bd, regsec, r.Scheme); err != nil {
			r.Log.Error(err, "set controller reference", "owner", bd.ObjectMeta, "controlled", regsec.ObjectMeta)
		}
		setAnnotation(bd, utils.AnnotationRobotSecretRef, regsec.Name)
		if err := r.update(ctx, bd); err != nil {
			return ctrl.Result{}, fmt.Errorf("update error: %w", err)
		}
	}

	// TODO: add conditions
	if bd.Status.Status != "ready" {
		bd.Status.Status = "ready"
		if bd.Status.Conditions == nil {
			bd.Status.Conditions = make([]goharborv1alpha1.Condition, 0)
		}
		if err := r.Status().Update(ctx, bd, &client.UpdateOptions{}); err != nil {
			if apierr.IsConflict(err) {
				log.Error(err, "failed to update status")
			} else {
				return ctrl.Result{}, err
			}
		}
	}

	// Loop
	return ctrl.Result{
		RequeueAfter: defaultCycle,
	}, nil
}

func (r *PullSecretBindingReconciler) update(ctx context.Context, binding *goharborv1alpha1.PullSecretBinding) error {
	if err := r.Client.Update(ctx, binding, &client.UpdateOptions{}); err != nil {
		return err
	}

	// Refresh object status to avoid problem
	namespacedName := types.NamespacedName{
		Name:      binding.Name,
		Namespace: binding.Namespace,
	}
	return r.Client.Get(ctx, namespacedName, binding)
}

func (r *PullSecretBindingReconciler) getConfigData(ctx context.Context, hsc *goharborv1alpha1.HarborServerConfiguration) (*model.HarborServer, error) {
	s := &model.HarborServer{
		ServerURL: hsc.Spec.ServerURL,
		InSecure:  hsc.Spec.InSecure,
	}

	namespacedName := types.NamespacedName{
		Namespace: hsc.Spec.AccessCredential.Namespace,
		Name:      hsc.Spec.AccessCredential.AccessSecretRef,
	}
	sec := &corev1.Secret{}
	if err := r.Client.Get(ctx, namespacedName, sec); err != nil {
		return nil, fmt.Errorf("failed to get the configured secret with error: %w", err)
	}

	cred := &model.AccessCred{}
	if err := cred.FillIn(sec); err != nil {
		return nil, fmt.Errorf("get credential error: %w", err)
	}

	s.AccessCred = cred

	return s, nil
}

func (r *PullSecretBindingReconciler) checkBindingRes(ctx context.Context, psb *goharborv1alpha1.PullSecretBinding) (*model.HarborServer, *corev1.ServiceAccount, ctrl.Result, error) {
	// Get server configuration
	hsc, err := r.getHarborServerConfig(ctx, psb.Spec.HarborServerConfig)
	if err != nil {
		// Retry later
		return nil, nil, ctrl.Result{}, fmt.Errorf("get server configuration error: %w", err)
	}

	if hsc == nil {
		// Not exist
		r.Log.Info("harbor server configuration does not exists", "name", psb.Spec.HarborServerConfig)
		// Do not need to reconcile again
		return nil, nil, ctrl.Result{}, nil
	}

	if hsc.Status.Status == defaultStatus || hsc.Status.Status == unhealthyStatus {
		return nil, nil, ctrl.Result{}, fmt.Errorf("status of Harbor server referred in configuration %s is unexpected: %s", hsc.Name, hsc.Status.Status)
	}

	// Get the specified service account
	sa, err := r.getServiceAccount(ctx, psb.Namespace, psb.Spec.ServiceAccount)
	if err != nil {
		// Retry later
		return nil, nil, ctrl.Result{}, fmt.Errorf("get service account error: %w", err)
	}

	if sa == nil {
		// Not exist
		r.Log.Info("service account does not exist", "name", psb.Spec.ServiceAccount)
		// Do not need to reconcile again
		return nil, nil, ctrl.Result{}, nil
	}

	hs, err := r.getConfigData(ctx, hsc)
	if err != nil {
		return nil, nil, ctrl.Result{}, fmt.Errorf("get config data error: %w", err)
	}

	return hs, sa, ctrl.Result{}, nil
}

func (r *PullSecretBindingReconciler) getHarborServerConfig(ctx context.Context, name string) (*goharborv1alpha1.HarborServerConfiguration, error) {
	hsc := &goharborv1alpha1.HarborServerConfiguration{}
	// HarborServerConfiguration is cluster scoped resource
	namespacedName := types.NamespacedName{
		Name: name,
	}
	if err := r.Client.Get(ctx, namespacedName, hsc); err != nil {
		// Explicitly check not found error
		if apierr.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	return hsc, nil
}

func (r *PullSecretBindingReconciler) getServiceAccount(ctx context.Context, ns, name string) (*corev1.ServiceAccount, error) {
	sc := &corev1.ServiceAccount{}
	namespacedName := types.NamespacedName{
		Namespace: ns,
		Name:      name,
	}

	if err := r.Client.Get(ctx, namespacedName, sc); err != nil {
		if apierr.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	return sc, nil
}

func (r *PullSecretBindingReconciler) createRegSec(ctx context.Context, namespace string, registry string, robot *model.Robot, psb *goharborv1alpha1.PullSecretBinding) (*corev1.Secret, error) {
	auths := &secret.Object{
		Auths: map[string]*secret.Auth{},
	}
	auths.Auths[registry] = &secret.Auth{
		Username: robot.Name,
		Password: robot.Token,
		Email:    fmt.Sprintf("%s@goharbor.io", robot.Name),
	}

	encoded := auths.Encode()

	regSec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RandomName("regsecret"),
			Namespace: namespace,
			Annotations: map[string]string{
				utils.AnnotationSecOwner: defaultOwner,
			},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: psb.APIVersion, Kind: psb.Kind, Name: psb.Name, UID: psb.UID}},
		},
		Type: regSecType,
		Data: map[string][]byte{
			datakey: encoded,
		},
	}

	return regSec, r.Client.Create(ctx, regSec, &client.CreateOptions{})
}

func (r *PullSecretBindingReconciler) deleteExternalResources(bd *goharborv1alpha1.PullSecretBinding) error {
	if pro, ok := bd.Annotations[utils.AnnotationProject]; ok {
		if err := r.HarborV2.DeleteProject(pro); err != nil {
			// TODO: handle delete error
			// Delete non-empty project will cause error?
			r.Log.Error(err, "delete external resources", "finalizer", finalizerID)
		}
	}

	return nil
}

func (r *PullSecretBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&goharborv1alpha1.PullSecretBinding{}).
		Complete(r)
}

func setAnnotation(obj *goharborv1alpha1.PullSecretBinding, key string, value string) {
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

	obj.Annotations[key] = value
}

func parseIntID(id string) int64 {
	intID, _ := strconv.ParseInt(id, 10, 64)
	return intID
}
