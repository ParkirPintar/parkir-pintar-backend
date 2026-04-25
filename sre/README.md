# sre

## Local Development (Docker Compose)

Docker Compose untuk spin up semua infra dependencies + services secara lokal. File: [`docker-compose.yaml`](./docker-compose.yaml)

### Quick Start

```bash
cd sre

# Start semua (infra + services)
docker compose up -d

# Atau infra only (jalankan services via `go run` di terminal masing-masing)
docker compose up -d postgres redis rabbitmq settlement-stub

# Check status
docker compose ps

# Tail logs
docker compose logs -f reservation billing

# Teardown + hapus volumes
docker compose down -v
```

### Services & Ports

| Service | Port | URL |
|---|---|---|
| PostgreSQL | 5432 | `postgres://parkir:parkir@localhost:5432/{db_name}` |
| Redis | 6379 | `localhost:6379` |
| RabbitMQ | 5672 / 15672 | AMQP: `localhost:5672`, Management UI: `http://localhost:15672` (guest/guest) |
| Settlement Stub (WireMock) | 8080 | `http://localhost:8080` |
| User Service | 50051 | `localhost:50051` (gRPC) |
| Reservation Service | 50052 | `localhost:50052` (gRPC) |
| Billing Service | 50053 | `localhost:50053` (gRPC) |
| Payment Service | 50054 | `localhost:50054` (gRPC) |
| Search Service | 50055 | `localhost:50055` (gRPC) |
| Presence Service | 50056 | `localhost:50056` (gRPC) |

### Databases

Satu PostgreSQL instance, 5 databases (dibuat otomatis via `init-db.sql`):

| Database | Service |
|---|---|
| `user_db` | User Service |
| `reservation_db` | Reservation Service + Search Service (read) |
| `billing_db` | Billing Service |
| `payment_db` | Payment Service |
| `analytics_db` | Analytics Service |

### Settlement Stub

WireMock stub untuk simulasi payment gateway (Pondo Ngopi). Mappings di `sre/stubs/`:
- `POST /v1/qris/create` → return QR code + payment_id
- `GET /v1/settlement/{id}` → return status PAID

### Infra-Only Mode

Kalau mau develop satu service dan run via `go run`:

```bash
# Start infra only
cd sre && docker compose up -d postgres redis rabbitmq settlement-stub

# Run service langsung
cd ../services/reservation
DATABASE_URL=postgres://parkir:parkir@localhost:5432/reservation_db REDIS_ADDR=localhost:6379 go run ./cmd
```

---

## API Documentation (Swagger)

OpenAPI 3.0 spec: [`e2e/swagger.yaml`](./e2e/swagger.yaml)

View locally:
```bash
npx @redocly/cli preview-docs e2e/swagger.yaml
# or
docker run -p 8080:8080 -e SWAGGER_JSON=/spec/swagger.yaml -v $(pwd)/e2e:/spec swaggerapi/swagger-ui
```

---

## Load Testing (k6)

Semua script ada di [`e2e/k6/`](./e2e/k6/).

### Install k6

```bash
# macOS
brew install k6

# Linux
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install k6
```

### Struktur File

```
e2e/k6/
├── helpers.js       # shared HTTP wrappers (auth, reserve, checkin, checkout, dll)
├── smoke-test.js    # 1 VU, 1 iterasi — sanity check semua endpoint
├── load-test.js     # full load test, 4 k6 scenarios (10–100 VUs)
├── war-booking.js   # stress test khusus war booking (100 VUs concurrent)
└── run-k6.sh        # runner script (smoke → load → war booking)
```

### Cara Jalankan

```bash
# Smoke test dulu (1 VU, pastikan semua endpoint OK)
k6 run --env BASE_URL=http://localhost:8000 e2e/k6/smoke-test.js

# Full load test (semua 14 scenario)
k6 run --env BASE_URL=http://localhost:8000 e2e/k6/load-test.js

# War booking stress test (100 VUs race untuk spot yang sama)
k6 run --env BASE_URL=http://localhost:8000 e2e/k6/war-booking.js

# Atau jalankan semua sekaligus
cd e2e/k6 && ./run-k6.sh http://localhost:8000

# Target production
cd e2e/k6 && ./run-k6.sh https://api.parkir-pintar.id
```

### Skenario Test

| # | Skenario | File | VUs | Durasi |
|---|---|---|---|---|
| S1 | Happy path system-assigned (Auth → Avail → Reserve → Checkin → Checkout → Pay) | `load-test.js` | 10 | 2m |
| S2 | Happy path user-selected (+ Hold step) | `load-test.js` | 5 | 2m |
| S3 | Double-book prevention — second request gets 409 | `load-test.js` | 0→100 | ramp |
| S4 | Spot contention / hold queue — 409 SPOT_HELD | `war-booking.js` | 0→100 | ramp |
| S5 | Reservation expiry no-show — GET → status EXPIRED | `load-test.js` | 5 | 2m |
| S6 | Wrong spot penalty — penalty_applied=200000 | `load-test.js` | 5 | 2m |
| S7 | Cancellation ≤ 2 min — cancellation_fee=0 | `load-test.js` | 5 | 2m |
| S8 | Cancellation > 2 min — cancellation_fee=5000 | `load-test.js` | 5 | 2m |
| S9 | Extended stay — no overstay penalty, standard hourly | `load-test.js` | 5 | 2m |
| S10 | Overnight fee — overnight_fee=20000 jika crossing midnight | `load-test.js` | 5 | 2m |
| S11 | Payment success QRIS — poll → status=PAID | `load-test.js` | 10 | 2m |
| S12 | Payment failure + retry — FAILED → retry → new QR code | `load-test.js` | 5 | 2m |
| S13 | Idempotency duplicate reservation — same Idempotency-Key → same reservation_id | `load-test.js` | 10 | 2m |
| S14 | Idempotency duplicate checkout — same Idempotency-Key → same invoice_id | `load-test.js` | 10 | 2m |

### Thresholds

| Metric | Threshold |
|---|---|
| `http_req_failed` | < 5% |
| `http_req_duration` p95 | < 3000ms |
| `reservation_duration_ms` p95 | < 2000ms |
| `checkout_duration_ms` p95 | < 2000ms |
| `double_book_prevented` | > 95% |
| `idempotency_correct` | > 99% |

### Catatan Test Environment

- **S5 (expiry)** dan **S8 (cancel > 2 min)** butuh env var yang diperpendek di service:
  ```
  RESERVATION_TTL=30s
  CANCEL_GRACE_PERIOD=10s
  ```
- **S10 (overnight fee)** butuh session yang crossing midnight — gunakan test env dengan clock manipulation atau mock billing service.
- `BASE_URL` default ke `http://localhost:8000` (Kong Gateway lokal).

---

## AWS Profile Setup

```bash
aws configure --profile terraform
aws sts get-caller-identity --profile terraform
```

## Terraform

```bash
cd terraform
terraform init
terraform plan -var github_org=<your-org> -var github_repo=<your-repo> ...
terraform apply -var github_org=<your-org> -var github_repo=<your-repo> ...
```

### Terraform Destroy (Teardown)

> **PENTING**: Jangan langsung `terraform destroy`. Hapus semua Kubernetes resources dulu, baru destroy infra. Kalau langsung destroy, EKS node masih hold ENI/ELB/SG yang bikin Terraform stuck atau error.

**Urutan teardown yang benar:**

```bash
# 1. Hapus semua workload di namespace parkir-pintar
kubectl delete namespace parkir-pintar --wait=true

# 2. Hapus observability stack
kubectl delete namespace monitoring --wait=true

# 3. Uninstall Istio (hapus CRDs, ingress gateway, istiod)
istioctl uninstall --purge -y
kubectl delete namespace istio-system --wait=true

# 4. Verify semua namespace sudah terminated
kubectl get namespaces
# Pastikan parkir-pintar, monitoring, istio-system sudah hilang

# 5. Hapus aws-auth configmap custom entries (optional, EKS akan di-destroy)
# kubectl edit configmap aws-auth -n kube-system  # revert manual changes

# 6. Baru destroy Terraform
cd terraform
terraform destroy -var github_org=<your-org> -var github_repo=<your-repo> ...
```

**Kenapa harus urutan ini?**

| Step | Alasan |
|---|---|
| Delete namespace dulu | Pod yang running hold ENI di subnet. Kalau subnet di-destroy duluan → ENI orphan → Terraform stuck |
| Uninstall Istio | Istio Ingress Gateway buat AWS ELB. Kalau ELB masih ada → SG dependency → VPC destroy gagal |
| Wait for termination | Kubernetes finalizer butuh waktu cleanup. `--wait=true` memastikan semua resource benar-benar hilang |
| Terraform destroy terakhir | Setelah semua K8s resources clean, Terraform bisa hapus EKS, RDS, ElastiCache, MQ, VPC tanpa conflict |

**Kalau Terraform destroy stuck:**

```bash
# Cek resource yang masih nempel
aws ec2 describe-network-interfaces --filters "Name=vpc-id,Values=<vpc-id>" --query 'NetworkInterfaces[*].{ID:NetworkInterfaceId,Status:Status,Description:Description}'

# Force detach ENI yang orphan
aws ec2 detach-network-interface --attachment-id <attachment-id> --force
aws ec2 delete-network-interface --network-interface-id <eni-id>

# Retry destroy
terraform destroy -var github_org=<your-org> -var github_repo=<your-repo> ...
```

## Post-Terraform Setup

Setelah `terraform apply` selesai, jalankan langkah-langkah berikut secara berurutan.

### 1. Update Kubeconfig

```bash
aws eks --region ap-southeast-1 update-kubeconfig --name parkirpintar --profile terraform

# Verify koneksi ke cluster
kubectl get nodes
```

### 2. Set Default StorageClass

EKS tidak otomatis set default StorageClass. Wajib dilakukan sebelum deploy apapun yang pakai PVC.

```bash
kubectl patch storageclass gp2 \
  -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'

# Verify — gp2 harus ada tanda (default)
kubectl get storageclass
```

### 3. Install Istio

```bash
# Download istioctl
curl -L https://istio.io/downloadIstio | sh -
cd istio-*/bin && export PATH=$PWD:$PATH

# Install ke cluster
istioctl install --set profile=default -y

# Verify semua pod running
kubectl get pods -n istio-system

# Ambil ELB endpoint — simpan untuk konfigurasi DNS
kubectl get svc -n istio-system istio-ingressgateway \
  -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'
```

### 4. Deploy Kubernetes Manifests

```bash
# Namespace harus di-apply duluan (kubectl apply -f dir/ diproses alfabetikal)
kubectl apply -f kubernetes/base/namespace.yaml
kubectl apply -f kubernetes/base/
kubectl apply -f kubernetes/istio/

# Verify
kubectl get pods -n parkir-pintar
```

### 5. Deploy Observability Stack

```bash
# Namespace dulu
kubectl apply -f observability/namespace.yaml
kubectl apply -f observability/

# Tunggu PVC Bound sebelum pod bisa running
kubectl get pvc -n monitoring -w

# Verify semua pod running
kubectl get pods -n monitoring
```

### 6. Tambahkan GitHub Actions Role ke aws-auth

Jalankan sekali agar GitHub Actions bisa `kubectl apply`:

```bash
kubectl edit configmap aws-auth -n kube-system
```

Tambahkan di bawah `mapRoles`:

```yaml
- rolearn: <github_actions_role_arn>   # dari: terraform output github_actions_role_arn
  username: github-actions
  groups:
    - system:masters
```

---

## Deploy App (manual)

K8s manifests are centralized in `sre/kubernetes/base/`. Sudah di-apply di [Post-Terraform Setup → Step 4](#4-deploy-kubernetes-manifests).

Untuk redeploy manual per service:
```bash
# apply satu service
kubectl apply -f kubernetes/base/reservation-service.yaml

# atau semua sekaligus
kubectl apply -f kubernetes/base/
```

## Check Deployment

```bash
kubectl get deployments -n parkir-pintar
kubectl get pods -n parkir-pintar
kubectl get svc -n parkir-pintar
```

## Get Credentials

RDS endpoints:
```bash
terraform output rds_user_endpoint
terraform output rds_reservation_endpoint
terraform output rds_billing_endpoint
terraform output rds_payment_endpoint
terraform output rds_analytics_endpoint
terraform output rds_reservation_replica_endpoint
```

Redis:
```bash
terraform output redis_endpoint
```

RabbitMQ:
```bash
terraform output rabbitmq_endpoint
```

## Observability

Stack: **Prometheus** (metrics) + **Grafana** (dashboard) + **Loki** (logs) + **Tempo** (traces) + **OTel Collector** (receiver).

Semua di-deploy ke namespace `monitoring` di EKS.

### Deploy

> Sudah tercakup di [Post-Terraform Setup → Step 5](#5-deploy-observability-stack). Bagian ini untuk redeploy manual.

```bash
kubectl apply -f observability/namespace.yaml
kubectl apply -f observability/
```

### Akses Grafana

```bash
# Via Istio Ingress Gateway (ELB)
http://<ELB_ENDPOINT>/monitor

# Via port-forward (lokal / troubleshooting)
kubectl port-forward svc/grafana 3000:3000 -n monitoring
# buka http://localhost:3000
```

Default credentials ada di `secrets.yaml` (`grafana-secret` → `admin-password`).

### Arsitektur Observability

| Komponen | Port | Fungsi |
|---|---|---|
| OTel Collector | 4317 (gRPC), 4318 (HTTP) | Receiver traces + metrics dari semua service |
| Prometheus | 9090 | Scrape metrics dari OTel Collector (`:8889`) + pod annotations |
| Loki | 3100 | Aggregasi logs dari Promtail |
| Tempo | 3200, 4317, 4318 | Penyimpanan traces dari OTel Collector |
| Grafana | 3000 | Dashboard — datasource: Prometheus, Loki, Tempo |
| Promtail | DaemonSet | Collect logs dari semua pod, kirim ke Loki |
| Beyla | DaemonSet | eBPF-based auto-instrumentation (zero-code tracing) |

### Flow Data

```
Service (zerolog + OTel SDK)
  │
  ├── traces  → OTel Collector :4317 → Tempo
  ├── metrics → OTel Collector :4317 → Prometheus (via :8889)
  └── logs    → stdout → Promtail → Loki
                                        │
                                    Grafana (unified dashboard)
```

### Check Status

```bash
kubectl get pods -n monitoring
kubectl logs -n monitoring deployment/otel-collector
kubectl logs -n monitoring deployment/prometheus
kubectl logs -n monitoring deployment/grafana
```

---

## GitHub Actions Setup

### Required Secrets

Set semua secret berikut di **GitHub → Repository → Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Cara Dapat | Dipakai di Job |
|---|---|---|
| `AWS_ROLE_ARN` | `terraform output github_actions_role_arn` | `build`, `deploy` |
| `SONAR_TOKEN` | [SonarCloud](https://sonarcloud.io) → My Account → Security → Generate Token | `verify-code-quality` |
| `GITHUB_TOKEN` | Otomatis tersedia, **tidak perlu di-set manual** | `verify-code-quality` |

> `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` **tidak dibutuhkan** — workflow pakai OIDC (`id-token: write`).

### Cara Ambil AWS_ROLE_ARN

```bash
cd sre/terraform
terraform output github_actions_role_arn
# contoh output: arn:aws:iam::123456789012:role/parkir-pintar-github-actions
```

### Cara Ambil SONAR_TOKEN

1. Login ke [sonarcloud.io](https://sonarcloud.io)
2. My Account → Security → Generate Tokens
3. Beri nama token (misal `parkir-pintar-ci`), klik Generate
4. Copy token → paste ke GitHub Secret `SONAR_TOKEN`

## Custom Domain (Cloudflare)

Arahkan ELB endpoint ke custom domain via Cloudflare:

1. Login ke [Cloudflare Dashboard](https://dash.cloudflare.com)
2. Pilih domain → **DNS** → **Records** → **Add Record**
3. Isi:
   - **Type**: `CNAME`
   - **Name**: subdomain yang diinginkan (misal `app`)
   - **Target**: ELB endpoint dari `kubectl get svc -o wide`
   - **Proxy status**: Proxied (orange cloud) untuk CDN + SSL, atau DNS only (grey cloud) untuk direct
4. Save

Untuk HTTPS, set **SSL/TLS** mode di Cloudflare ke **Flexible** atau **Full**.

### Cloudflare vs Route 53

| | Cloudflare | Route 53 |
|---|---|---|
| Harga | Free tier (DNS, CDN, SSL, DDoS) | $0.50/bulan per hosted zone + per query |
| CDN/WAF | Built-in gratis | Perlu CloudFront + AWS WAF (bayar) |
| DNS Routing | Basic | Advanced (latency, geolocation, failover) |
| AWS Integration | Manual | Native (Alias record, health check) |
| Best for | Hemat + butuh CDN/security gratis | Full AWS stack + advanced routing |

### Custom Domain via Route 53 (Terraform)

Buat file `route53.tf`:

```hcl
resource "aws_route53_zone" "main" {
  name = "domainmu.com"
}

resource "aws_route53_record" "app" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "app.domainmu.com"
  type    = "CNAME"
  ttl     = 300
  records = ["<ELB_ENDPOINT>"]
}
```

Lalu apply:
```bash
terraform apply
```

> **Note:** Kalau domain dibeli di luar AWS, update nameserver di registrar domain ke NS yang diberikan Route 53 di hosted zone.
