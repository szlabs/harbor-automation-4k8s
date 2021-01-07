# A registry CA populating sample

Use k8s daemonset to populate the registry CA to all the worker nodes:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: registry-ca
  namespace: kube-system
  labels:
    k8s-app: registry-ca
spec:
  selector:
    matchLabels:
      name: registry-ca
  template:
    metadata:
      labels:
        name: registry-ca
    spec:
      containers:
      - name: registry-ca
        image: busybox
        command: [ 'sh' ]
        args: [ '-c', 'cp /home/core/registry-ca /etc/docker/certs.d/my.custom.harbor/ca.crt && exec tail -f /dev/null' ]
        volumeMounts:
        - name: etc-docker
          mountPath: /etc/docker/certs.d/my.custom.harbor
        - name: ca-cert
          mountPath: /home/core
      terminationGracePeriodSeconds: 30
      volumes:
      - name: etc-docker
        hostPath:
          path: /etc/docker/certs.d/my.custom.harbor
      - name: ca-cert
        secret:
          secretName: registry-ca
```