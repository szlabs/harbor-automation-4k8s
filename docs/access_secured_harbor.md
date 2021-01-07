# Access secured Harbor

## Configure containerd 
Configure [contained](https://github.com/containerd/cri/blob/master/docs/registry.md#configure-registry-tls-communication) to 
access your secured Harbor registry by appending the following settings into contained configuration file `/etc/containerd/config.toml`:

```
# explicitly use v2 config format
version = 2

# The registry host has to be a domain name or IP. Port number is also
# needed if the default HTTPS or HTTP port is not used.
[plugin."io.containerd.grpc.v1.cri".registry.configs."my.custom.harbor".tls]
    ca_file   = "<harbor ca path>"
    # cert_file = "<harbor server cert path>"
    # key_file  = "<harbor server key path>"
```

Configure containerd to skip the key verification by applying the following settings:

```
# explicitly use v2 config format
version = 2

[plugin."io.containerd.grpc.v1.cri".registry.configs."my.custom.harbor".tls]
    insecure_skip_verify = true
```

## Configure Kind
This section will show you how to configure your [kind](https://kind.sigs.k8s.io/docs/user/private-registries/#use-a-certificate) cluster 
with [contained](https://github.com/containerd/containerd) runtime to access the secured [Harbor](https://goharbor.io) registry. 

1. Put your harbor ca cert into a folder <ca_folder_path>
1. Add extra mound configuration to all the worker nodes and/or control-plane nodes in kind configuration file:
   
    ```
    nodes:
    - role: control-plane
      extraMounts:
      - containerPath: <ca_folder_path>
        hostPath: <host_ca_folder_path>
    - role: worker
      extraMounts:
      - containerPath: <ca_folder_path>
        hostPath: <host_ca_folder_path>
    ```

1. Add extra containerd config patch

    ```
    containerdConfigPatches:
    - |-
      [plugins."io.containerd.grpc.v1.cri".registry.configs."my.custom.harbor".tls]
        ca_file = "<ca_folder_path>"
    ```

An overall [1 control-plane node and 3 worker nodes kind] config example:

```yaml
# a cluster with 3 control-plane nodes and 3 workers
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.configs."my.custom.harbor".tls]
    ca_file = "<ca_folder_path>"
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
  extraMounts:
  - containerPath: <ca_folder_path>
    hostPath: <host_ca_folder_path>
- role: worker
  extraMounts:
  - containerPath: <ca_folder_path>
    hostPath: <host_ca_folder_path>
- role: worker
  extraMounts:
  - containerPath: <ca_folder_path>
    hostPath: <host_ca_folder_path>
- role: worker
  extraMounts:
  - containerPath: <ca_folder_path>
    hostPath: <host_ca_folder_path>
```