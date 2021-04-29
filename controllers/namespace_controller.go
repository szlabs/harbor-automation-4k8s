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

	"github.com/szlabs/harbor-automation-4k8s/pkg/utils"

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
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/legacy"
	v2 "github.com/szlabs/harbor-automation-4k8s/pkg/rest/v2"
)

const (
	annotationIssuer  = "goharbor.io/secret-issuer"
	annotationAccount = "goharbor.io/service-account"
	defaultSaName     = "default"
)

// NamespaceReconciler reconciles a Namespace object
type NamespaceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	HarborV2 *v2.Client
	Harbor   *legacy.Client
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

	// Pull secret issuer is set and then check if the required default binding exists
	// Confirm the service account name
	// Use default SA if not set inside annotation
	saName := defaultSaName
	if setSa, ok := ns.Annotations[annotationAccount]; ok {
		// Check if custom service account exist
		sa := &corev1.ServiceAccount{}
		saNamespacedName := types.NamespacedName{
			Namespace: ns.Name,
			Name:      saName,
		}
		if err := r.Client.Get(ctx, saNamespacedName, sa); err != nil {
			if apierr.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("service account %s not found in namespace %s: %w", setSa, ns.Name, err)
			}
			return ctrl.Result{}, fmt.Errorf("get service account %s in namespace %s error: %w", setSa, ns.Name, err)
		}
		saName = setSa
	}

	// Find PSB
	for _, bd := range bindings.Items {
		if bd.Spec.HarborServerConfig == harborCfg && bd.Spec.ServiceAccount == saName {
			// Found it and reconcile is done
			return ctrl.Result{}, nil
		}
	}

	// Validate the annotation and create PSB is needed
	proj, projExist := ns.Annotations[annotationProject]
	robot, robotExist := ns.Annotations[annotationRobot]

	// Skip PSB creation if project and robot don't match
	if projExist != robotExist {
		// TODO: refine logging
		log.Error(fmt.Errorf("project: %s, robot: %s", proj, robot), "Harbor annotation for project and robot doesn't match")
		return ctrl.Result{}, nil
	}

	// Validate project and robot if both non-empty
	if projExist {
		err := r.validateProjectRobot(proj, robot)
		if err != nil {
			log.Error(err, "Harbor annotation for project and robot invalid", "project", proj, "robot", robot)
			return ctrl.Result{}, nil
		}
	}

	// Automatically generate project and robot account based on namespace name
	if !projExist {
		proj = utils.RandomName(ns.Name)
		robotID, err := r.createProjectRobot(proj)
		if err != nil {
			log.Error(err, "Failed creating project and robot", "project", proj, "robot", robot)
			return ctrl.Result{}, nil
		}
		robot = robotID
	}

	// PSB doesn't exist, create one
	defaultBinding := r.getNewBindingCR(ns.Name, harborCfg, saName)
	if err := controllerutil.SetControllerReference(ns, defaultBinding, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("set ctrl reference error: %w", err)
	}

	defaultBinding.Spec.Robot = robot
	defaultBinding.Spec.Project = proj

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

func (r *NamespaceReconciler) validateProjectRobot(proj, robot string) error {
	robotID, err := strconv.ParseInt(robot, 10, 64)
	if err == nil {
		return err
	}
	projID, err := r.HarborV2.EnsureProject(proj)
	if err != nil {
		return err
	}
	_, err = r.Harbor.GetRobotAccount(projID, robotID)
	return err
}

func (r *NamespaceReconciler) createProjectRobot(proj string) (string, error) {
	projID, err := r.HarborV2.EnsureProject(proj)
	if err != nil {
		return "", err
	}
	robot, err := r.Harbor.CreateRobotAccount(projID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", robot.ID), nil
}
