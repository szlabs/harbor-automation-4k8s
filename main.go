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

package main

import (
	"flag"
	"os"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	_ "sigs.k8s.io/controller-tools/pkg/crd"

	goharborv1alpha1 "github.com/szlabs/harbor-automation-4k8s/api/v1alpha1"
	"github.com/szlabs/harbor-automation-4k8s/controllers"
	"github.com/szlabs/harbor-automation-4k8s/pkg/rest/legacy"
	v2 "github.com/szlabs/harbor-automation-4k8s/pkg/rest/v2"
	"github.com/szlabs/harbor-automation-4k8s/webhooks/hsc"
	"github.com/szlabs/harbor-automation-4k8s/webhooks/pod"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("harbor-automation-4k8s")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = goharborv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "bbdd0aed.goharbor.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.HarborServerConfigurationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("HarborServerConfiguration"),
		Scheme: mgr.GetScheme(),
		Harbor: legacy.New(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HarborServerConfiguration")
		os.Exit(1)
	}
	if err = (&controllers.NamespaceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Namespace")
		os.Exit(1)
	}
	if err = (&controllers.PullSecretBindingReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("PullSecretBinding"),
		Scheme:   mgr.GetScheme(),
		HarborV2: v2.New(),
		Harbor:   legacy.New(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PullSecretBinding")
		os.Exit(1)
	}

	// Add webhook
	mgr.GetWebhookServer().Register("/mutate-image-path", &webhook.Admission{
		Handler: &pod.ImagePathRewriter{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("webhooks").WithName("MutatingImagePath"),
		}})
	mgr.GetWebhookServer().Register("/validate-hsc", &webhook.Admission{
		Handler: &hsc.Validator{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("webhooks").WithName("HarborServerConfigurationValidator"),
		}})
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
