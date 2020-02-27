# Using Nginx Ingress Controller in Istio

## Create Cluster
```shell script
gcloud container clusters create -m n1-standard-2 ingress-test
```
## Download istio release
Istio release version 1.4.2 is used for the exercise
```shell script
export ISTIO_VERSION=1.4.2; curl -L https://istio.io/downloadIstio | sh -
```
## Deploy Istio
1. Deploy istio components using the [istio operator api](https://istio.io/blog/2019/introducing-istio-operator/)
Note the usage of `--set` flags to enable mtls and controlPlaneSecurity in the istio mesh to deploy
```shell script
./istio-1.4.2/bin/istioctl manifest apply \
	--set values.global.mtls.enabled=true \
	--set values.global.controlPlaneSecurityEnabled=true
```
2. check statuses of all istio pods and wait for all istio components to be ready
```shell script
kubectl get pods -n istio-system
```
the expected output should look like:
```shell script
NAME                                      READY   STATUS    RESTARTS   AGE
istio-citadel-6dc789bc4c-chdkb            1/1     Running   0          104s
istio-galley-78989484fd-przvl             2/2     Running   0          103s
istio-ingressgateway-7c6c65958-lv8kh      1/1     Running   0          103s
istio-pilot-66bf767468-5r94m              2/2     Running   0          103s
istio-policy-84667d9ff8-hhzw7             2/2     Running   1          104s
istio-sidecar-injector-7d65c79dd5-7dtrg   1/1     Running   0          102s
istio-telemetry-5dd48bb8bc-24m8d          2/2     Running   2          103s
prometheus-586d4445c7-22vhb               1/1     Running   0          103s
```

## Deploy Sample httpbin application
```shell script
kubectl label namespace default istio-injection=enabled
kubectl apply -f ./istio-1.4.2/samples/httpbin/httpbin.yaml
```

## Set up nginx ingress proxy in the default namespace
```shell script
export KUBE_API_SERVER_IP=$(kubectl get svc kubernetes -o jsonpath='{.spec.clusterIP}')/32
sed "s#__KUBE_API_SERVER_IP__#${KUBE_API_SERVER_IP}#" nginx-default-ns.yaml | kubectl apply -f -
```

## Set up nginx ingress proxy in the ingress namespace
1. Deploy nginx-ingress-controller and its default backend
```shell script
kubectl create namespace ingress
kubectl label namespace ingress istio-injection=enabled
sed "s#__KUBE_API_SERVER_IP__#${KUBE_API_SERVER_IP}#" nginx-ingress-ns.yaml | kubectl apply -f -
```
2. Create ingress resource routing to httpbin
```shell script
kubectl apply -f ingress-ingress-ns.yaml
```
3. Create sidecar resource to allow traffic from pods in ingress namespace to pods in default namespace
```shell script
kubectl apply -f sidecar-ingress-ns.yaml
```

## Verify that external traffic can be routed to the httpbin service from the nginx ingress in both the default and the ingress namespaces
1. Verify that external traffic can be routed to the httpbin service from the nginx ingress proxy in the default namespace
```shell script
curl $(kubectl get svc -n default ingress-nginx -o jsonpath='{.status.loadBalancer.ingress[0].ip}')/ip -v
```
2. Verify that external traffic can be routed to the httpbin service from the nginx ingress proxy in the ingress namespace
```shell script
curl $(kubectl get svc -n ingress ingress-nginx -o jsonpath='{.status.loadBalancer.ingress[0].ip}')/ip -v
```
3. The expected responses for both curl requests should look like:
```shell script
*   Trying 34.83.167.92...
* TCP_NODELAY set
* Connected to 34.83.167.92 (34.83.167.92) port 80 (#0)
> GET /ip HTTP/1.1
> Host: 34.83.167.92
> User-Agent: curl/7.58.0
> Accept: */*
> 
< HTTP/1.1 200 OK
< Server: nginx/1.13.9
< Date: Mon, 17 Feb 2020 21:06:18 GMT
< Content-Type: application/json
< Content-Length: 30
< Connection: keep-alive
< access-control-allow-origin: *
< access-control-allow-credentials: true
< x-envoy-upstream-service-time: 2
< 
{
  "origin": "10.138.0.13"
}
* Connection #0 to host 34.83.167.92 left intact
```

## Verify that the httpbin service does not receive traffic in plaintext
1. Deploy sleep application outside the istio mesh (without sidecar) and wait for sleep pods to be ready
```shell script
kubectl create namespace legacy
kubectl apply -f ./istio-1.4.2/samples/sleep/sleep.yaml -n legacy
kubectl get pods -n legacy
```
The expected output should look something like:
```shell script
NAME                     READY   STATUS    RESTARTS   AGE
sleep-666475687f-wx2xp   1/1     Running   0          37s
```
2. Curl httpbin service from the sleep pod
```shell script
kubectl exec -it $(kubectl get pod -n legacy -l app=sleep -o jsonpath='{.items[0].metadata.name}') -n legacy -- curl httpbin.default.svc.cluster.local:8000/ip -v
```
3. The expected output should look like:
```shell script
* Expire in 0 ms for 6 (transfer 0x55d92c811680)
......
* Expire in 15 ms for 1 (transfer 0x55dc9cca6680)
*   Trying 10.15.247.45...
* TCP_NODELAY set
* Expire in 200 ms for 4 (transfer 0x55dc9cca6680)
* Connected to httpbin.default.svc.cluster.local (10.15.247.45) port 8000 (#0)
> GET /ip HTTP/1.1
> Host: httpbin.default.svc.cluster.local:8000
> User-Agent: curl/7.64.0
> Accept: */*
> 
* Recv failure: Connection reset by peer
* Closing connection 0
curl: (56) Recv failure: Connection reset by peer
command terminated with exit code 56
```

## Verify that the connection between nginx-ingress-controller and httpbin service are mtls enabled
1. Use `istioctl` to verify the authentication policies between the httpbin service and nginx-ingress pods
```shell script
./istio-1.4.2/bin/istioctl authn tls-check $(kubectl get pod -n default -l app=ingress-nginx -o jsonpath='{.items[0].metadata.name}') httpbin.default.svc.cluster.local
./istio-1.4.2/bin/istioctl authn tls-check -n ingress $(kubectl get pod -n ingress -l app=ingress-nginx -o jsonpath='{.items[0].metadata.name}') httpbin.default.svc.cluster.local
```
2. The expected output for both should look like:
```shell script
HOST:PORT                                  STATUS     SERVER     CLIENT           AUTHN POLICY     DESTINATION RULE
httpbin.default.svc.cluster.local:8000     OK         STRICT     ISTIO_MUTUAL     /default         istio-system/default
```