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

	harborClient "github.com/szlabs/harbor-automation-4k8s/pkg/controllers/harbor"
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
	v2models "github.com/szlabs/harbor-automation-4k8s/pkg/sdk/harbor_v2/models"
)

const (
	defaultSaName = "default"
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
			// The resource may have been deleted after reconcile request coming in
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

	// If auto is set for image rewrite rule
	harborCfg, err := r.findDefaultHarborCfg(ctx, log, ns)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error finding harborCfg: %w", err)
	}

	if harborCfg == nil {
		log.Info("no default hsc for this namespace, skip PSB creation")
		r.removeStalePSB(ctx, log, bindings)
		return ctrl.Result{}, nil
	}

	// Pull secret issuer is set and then check if the required default binding exists
	// Confirm the service account name
	// Use default SA if not set inside annotation
	saName := defaultSaName
	if setSa, ok := ns.Annotations[utils.AnnotationAccount]; ok {
		saName = setSa
	}
	// Check if custom service account exist
	sa := &corev1.ServiceAccount{}
	saNamespacedName := types.NamespacedName{
		Namespace: ns.Name,
		Name:      saName,
	}
	if err := r.Client.Get(ctx, saNamespacedName, sa); err != nil {
		if apierr.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("service account %s not found in namespace %s: %w", saName, ns.Name, err)
		}
		return ctrl.Result{}, fmt.Errorf("get service account %s in namespace %s error: %w", saName, ns.Name, err)
	}

	// Find PSB
	for _, bd := range bindings.Items {
		if bd.Spec.HarborServerConfig == harborCfg.Name && bd.Spec.ServiceAccount == saName {
			// Found it and reconcile is done
			// TODO: the PSB might be useless if the credentials or projects are changed
			// Need to check if the PSB is still valid.
			log.Info("psb exist for this namespace")
			return ctrl.Result{}, nil
		}
	}

	proj, projExist := ns.Annotations[utils.AnnotationProject]
	if !projExist || proj == "" {
		log.Info("annotation 'project' not set, skip reconciliation")
		return ctrl.Result{}, nil
	}

	if err := r.setHarborClient(ctx, log, harborCfg); err != nil {
		return ctrl.Result{}, err
	}

	var projName, projID, robotID string
	if projName, projID, robotID, err = r.validateHarborProjectAndRobot(ctx, log, ns); err != nil {
		return ctrl.Result{}, err
	}

	// PSB doesn't exist, create one
	log.Info("creating pull secret binding")
	psb, err := r.createPullSecretBinding(ctx, ns, harborCfg.Name, saName, robotID, projID)
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info("created pull secret binding", "name", psb.Name)

	// update namespace with updated annotation
	if proj == "*" {
		log.Info("update namespace annotations", "projectName", projName, "robotID", robotID)
		ns.Annotations[utils.AnnotationProject] = projName
		ns.Annotations[utils.AnnotationRobot] = robotID
		if err := r.Client.Update(ctx, ns, &client.UpdateOptions{}); err != nil {
			return ctrl.Result{}, err
		}
	}

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

func (r *NamespaceReconciler) validateProject(projectName string) (string, error) {
	var proj *v2models.Project
	var err error
	if proj, err = r.HarborV2.GetProject(projectName); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", proj.ProjectID), nil
}

func (r *NamespaceReconciler) validateRobot(proj, robot string) error {
	if robot == "" {
		return fmt.Errorf("robot should not be empty")
	}
	if proj == "" {
		return fmt.Errorf("proj should not be empty")
	}

	robotID, err := strconv.ParseInt(robot, 10, 64)
	if err != nil {
		return err
	}

	projectID, err := strconv.ParseInt(proj, 10, 64)
	if err != nil {
		return err
	}

	_, err = r.Harbor.GetRobotAccount(projectID, robotID)
	return err
}

func (r *NamespaceReconciler) createProjectAndRobot(proj string) (string, string, error) {
	projID, err := r.HarborV2.EnsureProject(proj)
	if err != nil {
		return "", "", err
	}

	robot, err := r.Harbor.CreateRobotAccount(projID)
	if err != nil {
		return "", "", err
	}

	return fmt.Sprintf("%d", projID), fmt.Sprintf("%d", robot.ID), nil
}

func (r *NamespaceReconciler) findDefaultHarborCfg(ctx context.Context, log logr.Logger, ns *corev1.Namespace) (*goharborv1alpha1.HarborServerConfiguration, error) {
	// check annotation first
	harborCfg, yes := ns.Annotations[utils.AnnotationHarborServer]
	if yes && harborCfg != "" {
		hsc := &goharborv1alpha1.HarborServerConfiguration{}
		err := r.Client.Get(ctx, types.NamespacedName{Name: harborCfg}, hsc)
		if err != nil {
			if apierr.IsNotFound(err) {
				log.Info("hsc specified in annotation doesn't exist")
				return nil, nil
			}

			return nil, fmt.Errorf("error when finding hsc specified in annotation: %w", err)
		}
		return hsc, nil
	}
	log.Info("no default hsc found in annotation")

	// then find global default hsc
	hscs := &goharborv1alpha1.HarborServerConfigurationList{}
	err := r.Client.List(ctx, hscs)
	if err != nil {
		return nil, fmt.Errorf("error listing harborCfg: %w", err)
	}

	if len(hscs.Items) > 0 {
		for _, hsc := range hscs.Items {
			if hsc.Spec.Default {
				log.Info("found global default hsc: " + hsc.Name)
				return &hsc, nil
			}
		}
	}
	return nil, nil
}

func (r *NamespaceReconciler) removeStalePSB(ctx context.Context, log logr.Logger, bindings *goharborv1alpha1.PullSecretBindingList) error {
	if len(bindings.Items) > 0 {
		log.Info("removig stale psb in namespace")
		for _, bd := range bindings.Items {
			// Remove all the existing bindings as issuer is removed
			if err := r.Client.Delete(ctx, &bd, &client.DeleteOptions{}); err != nil {
				// Retry next time
				return fmt.Errorf("remove binding %s error: %w", bd.Name, err)
			}
		}
	}
	return nil
}

func (r *NamespaceReconciler) createPullSecretBinding(ctx context.Context, ns *corev1.Namespace, harborCfg, saName, robotID, projID string) (*goharborv1alpha1.PullSecretBinding, error) {
	defaultBinding := r.getNewBindingCR(ns.Name, harborCfg, saName)
	if err := controllerutil.SetControllerReference(ns, defaultBinding, r.Scheme); err != nil {
		return nil, fmt.Errorf("set ctrl reference error: %w", err)
	}

	defaultBinding.Spec.RobotID = robotID
	defaultBinding.Spec.ProjectID = projID

	if err := r.Client.Create(ctx, defaultBinding, &client.CreateOptions{}); err != nil {
		return nil, fmt.Errorf("create binding CR error: %w", err)
	}

	return defaultBinding, nil
}

func (r *NamespaceReconciler) setHarborClient(ctx context.Context, log logr.Logger, harborCfg *goharborv1alpha1.HarborServerConfiguration) error {
	// Create harbor client
	harborV2, harborLegacy, err := harborClient.CreateHarborClients(ctx, r.Client, harborCfg)
	if err != nil {
		log.Error(err, "failed to create harbor client")
		return err
	}
	r.HarborV2 = harborV2
	r.Harbor = harborLegacy
	return nil
}

func (r *NamespaceReconciler) validateHarborProjectAndRobot(ctx context.Context, log logr.Logger, ns *corev1.Namespace) (string, string, string, error) {
	var err error
	var projID string

	// Validate the annotation and create PSB is needed
	proj := ns.Annotations[utils.AnnotationProject]
	robotID, robotExist := ns.Annotations[utils.AnnotationRobot]

	if proj == "*" {
		log.Info("validate project and robot account")
		// Automatically generate project and robot account based on namespace name
		// TODO: should be more structure name since many clusters might share the same Harbor instance
		proj = utils.RandomName(ns.Name)
		projID, robotID, err = r.createProjectAndRobot(proj)
		if err != nil {
			log.Error(err, "Failed creating project and robot", "project", proj, "robot", robotID)
			return "", "", "", err
		}
		return proj, projID, robotID, nil
	} else {
		projID, err = r.validateProject(proj)
		if err != nil {
			log.Error(err, "Harbor annotation for project is invalid", "project", proj)
			return "", "", "", fmt.Errorf("project are invalid: %w", err)
		}
		if !robotExist {
			return "", "", "", fmt.Errorf("robotID is not set: %w", err)
		}

		err := r.validateRobot(projID, robotID)
		if err != nil {
			log.Error(err, "annotation 'robotID'  is invalid", "robotID", robotID)
			return "", "", "", fmt.Errorf("robotID is invalid: %w", err)
		}
	}

	return proj, projID, robotID, nil
}
