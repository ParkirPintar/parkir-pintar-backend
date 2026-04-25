# Setup Kubernetes Cluster

## Install Kong Ingress Controller

    Install the Gateway API CRDs before installing Kong Ingress Controller.
    ```bash
    kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml
    ```

    ```bash
    echo "
    ---
    apiVersion: gateway.networking.k8s.io/v1
    kind: GatewayClass
    metadata:
    name: kong
    annotations:
    konghq.com/gatewayclass-unmanaged: 'true'

    spec:
    controllerName: konghq.com/kic-gateway-controller
    ---
    apiVersion: gateway.networking.k8s.io/v1
    kind: Gateway
    metadata:
    name: kong
    spec:
    gatewayClassName: kong
    listeners:
    - name: proxy
    port: 80
    protocol: HTTP
    " | kubectl apply -f -
    ```
    Install Kong Ingress Controller

    ```bash
    helm repo add kong https://charts.konghq.com
    helm repo update
    ```

    ```bash
    helm install kong kong/ingress -n kong --create-namespace 
    ```
