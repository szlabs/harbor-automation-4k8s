# harbor-automation-4k8s

harbor-automation-4k8 provides the following features to help users get better experiences of using [Harbor](https://github.com/goharbor/harbor)
or apply some Day2 operations to the Harbor deployed in the Kubernetes cluster:

- [x] **Mapping k8s namespace and harbor project**: make sure there is a relevant project existing at linked Harbor side for the
 specified k8s namespace pulling image from there (bind specified one or create new).
- [x] **Pulling secret auto injection**: auto create robot account in the corresponding Harbor project and bind it to the
 related service account of the related k8s namespace to avoid explicitly specifying image pulling secret in the
 deployment manifest yaml.
 - [x] **image path auto rewriting**: rewrite the pulling path of the matched workload images (e.g: no full repository path specified)
 being deployed in the specified k8s namespace to the corresponding project at the linked Harbor.
- [x] **transparent proxy cache**: rewrite the pulling path of the matched workload images to the proxy cache project of the linked Harbor.
- [ ] apply configuration changes: update the system configurations of the linked Harbor with Kubernetes way by providing a configMap.
- [ ] certificate population: populate the CA of the linked Harbor instance to the container runtimes of all the cluster workers and let workers trust it to avoid image pulling issues.
- [ ] TBD

## Overall Design

The diagram below shows the overall design of this project:

![overall design](./docs/assets/4k8s-automation.png)

* A cluster scoped Kubernetes CR named `HarborServerConfiguration` is designed to keep the Harbor server access info by providing the access
host and access key & secret (key and secret should be wrapped into a kubernetes secret) for future referring.
* To enable the image pull secret injection in a Kubernetes namespace:
  - add the annotation `goharbor.io/harbor:[harborserverconfiguration_cr_name]` to the namespace. `harborserverconfiguration_cr_name`
  is the name of the CR `HarborServerConfiguration` that includes the Harbor server info.
  - add the annotation `goharbor.io/service-account:[service_account_name]` to the namespace. `service_account_name` is the
  name of the Kubernetes service account that you want to use to bind the image pulling secret later.
* When the namespace is created, the operator will check the related annotations set above. If they're set, then:
  - ensures a corresponding harbor project exists (or creates one if none exists) at the Harbor referred by
  the `HarborServerConfiguration` referred in `goharbor.io/harbor`.
  - ensures a robot account under the mapping project exists (or creates one if none exists).
  - a CR `PullSecretBinding` is created to keep the relationship between Kubernetes resources and Harbor resources.
  - the mapping project is recorded in annotation `annotation:goharbor.io/project` of the CR `PullSecretBinding`.
  - the linked robot account is recorded in annotation `annotation:goharbor.io/robot` of the CR `PullSecretBinding`.
  - make sure the linked robot account is wrapped as a Kubernetes secret and bind with the service account that is
  specified in the annotation `annotation:goharbor.io/service-account` of the namespace.
* Now `annotation:goharbor.io/image-rewrite` has three kinds of value.
    * `auto`, the mutating webhook is enabled. Controller will create project and robot specified inside namespace if it doesn't exist. If there is no default global HSC or no harbor specified, no PSB will be created for current namespace
    * `global` the mutating webhook is enabled. Controller will throw error if project specified inside namespace doesn't exist. It will create robot account if it doesn't exist. The controller will use the harbor in assign HSC first. If it does not exist, use the harbor in global default HSC
    * not set, the mutating webhook is disabled.
  - any pods deployed to the namespace with image that does not have registry host (e.g.: `nginx:1.14.3`) will be rewrite
  by adding harbor host and mapping project (e.g.: `goharbor.io/namespace1_xxx/nginx:1.14.3`) from the `HarborServerConfiguration`
  referred in `goharbor.io/harbor`.
* tbd

## Installation

For trying this project, you can follow the guideline shown below to quickly install the operator and webhook to your cluster:
(tools like `git`,`make`,`kustomize` and `kubectl` should be installed and available in the $PATH)

1. Clone the repository

```shell script
git clone git@github.com:szlabs/harbor-automation-4k8s.git
```

1. Build the image (no official image built so far)

```shell script
# set the image path
export IMG=goharbor.io/goharbor/harbor-automation-4k8s:dev

make docker-build && make docker-push
```

1. Deploy the operator to the Kubernetes cluster

```shell script
make deploy
```

2. Check the status of the operator

```
kubectl get all -n harbor-automation-4k8s-system
```

3. Uninstall the operator

```shell script
kustomize build config/default | kubectl delete -f -
```
## Usages

### HarborServerConfiguration CR

Register your Harbor in a `HarborServerConfiguration` CR:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
  namespace: kube-system
type: Opaque
data:
  accessKey: YWRtaW4=
  accessSecret: SGFyYm9yMTIzNDU=
---
apiVersion: goharbor.goharbor.io/v1alpha1
kind: HarborServerConfiguration
metadata:
  name: harborserverconfiguration-sample
spec:
  default: true ## whether it will be default global hsc
  serverURL: 10.168.167.189
  accessCredential:
    namespace: kube-system
    accessSecretRef: mysecret
  version: 2.1.0
  inSecure: true
  rules: ## rules to define to rewrite image path
  - "docker.io,myharbor"    ## <repo-regex>,<harbor-project>
  namespaceSelector:
    matchLabels:
      usethisHSC: true
```

Create it:

```shell script
kubectl apply -f hsc.yaml
```

Use the following command to check the `HarborServerConfiguration` CR (short name: `hsc`):

```shell script
kubectl get hsc
```

### Pulling secret injection

Add related annotations to your namespace when enabling secret injection:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: sz-namespace1
  annotations:
    goharbor.io/harbor: harborserverconfiguration-sample
    goharbor.io/service-account: default
    goharbor.io/project: "*"
```

Create it:

```shell script
kubectl apply -f namespace.yaml
```

After the automation is completed, a CR `PullSecretBinding` is created:

```shell script
kubectl get psb -n sz-namespace1

# output
#NAME             HARBOR SERVER                      SERVICE ACCOUNT   STATUS
#binding-txushc   harborserverconfiguration-sample   default           ready
```

Get the details of the psb/binding-xxx:

```shell script
k8s get psb/binding-txushc -n sz-namespace1 -o yaml
```

Output details:

```yaml
apiVersion: goharbor.goharbor.io/v1alpha1
kind: PullSecretBinding
metadata:
  annotations:
    goharbor.io/project: sz-namespace1-axtnd8
    goharbor.io/robot: "31"
    goharbor.io/robot-secret: regsecret-sab3pq
  creationTimestamp: "2020-12-02T15:21:48Z"
  finalizers:
  - psb.finalizers.resource.goharbor.io
  generation: 1
  name: binding-txushc
  namespace: sz-namespace1
  ownerReferences:
  - apiVersion: v1
    blockOwnerDeletion: true
    controller: true
    kind: Namespace
    name: sz-namespace1
    uid: 810efadd-b560-4791-8007-8decaf2fbb1c
  resourceVersion: "2500851"
  selfLink: /apis/goharbor.goharbor.io/v1alpha1/namespaces/sz-namespace1/pullsecretbindings/binding-txushc
  uid: f5b4f68a-4657-4f89-b231-0fc96c03ca00
spec:
  harborServerConfig: harborserverconfiguration-sample
  serviceAccount: default
status:
  conditions: []
  status: ready
```

The related auto-generated data is recorded in the related annotations:

```yaml
annotations:
  goharbor.io/project: sz-namespace1-axtnd8
  goharbor.io/robot: "31"
  goharbor.io/robot-secret: regsecret-sab3pq
```

### Image path rewrite

To enable image rewrite, set the rules section in hsc, or set annotation to refer to a configMap that contains rules and hsc

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: sz-namespace1
  annotations:
    goharbor.io/harbor: harborserverconfiguration-sample
    goharbor.io/service-account: default
    goharbor.io/rewriting-rules: sz-namespace1
```

Corresponding ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sz-namespace1
  namespace: sz-namespace1
data:
  hsc: harbor2
  rewriting: "on"
  rules: | # configMap doesn't support storing nested string
    docker.io,highestproject
    gcr.io,a

```

Corresponding HSC

```yaml
apiVersion: goharbor.goharbor.io/v1alpha1
kind: HarborServerConfiguration
metadata:
  name: harborserverconfiguration-sample
spec:
  serverURL: 10.168.167.12
  accessCredential:
    namespace: kube-system
    accessSecretRef: mysecret
  version: 2.1.0
  inSecure: true
  rules: ## rules to define to rewrite image path
  - "docker.io,testharbor"    ## <repo-regex>,<harbor-project>

```

As mentioned before, the mutating webhook will rewrite all the images of the deploying pods which has no registry host
prefix to the flowing pattern:

`image:tag => <hsc/hsc-name.[spec.serverURL]>/<psb/binding-xxx.[metadata.annotations[goharbor.io/project]]>/image:tag`
