## Traffic Routing

Primary endpoints:
- app.kisskissplay.com — web application
- api-prod.kisskissplay.com — API
- ws-prod.kisskissplay.com — WebSocket
- api-ws-prod.kisskissplay.com — WebSocket API
- public-prod.kisskissplay.com — public static storage
- private-prod.kisskissplay.com — private static storage
- assets-prod.kisskissplay.com — public assets storage

### DNS
The `kisskissplay.com` domain zone is managed by Gcore Managed Geo DNS. Clients from Russia are routed to Yandex Cloud; clients from the rest of the world are routed to DigitalOcean.
DNS zone configuration: https://github.com/playneta/kiss2-infra/tree/main/terraform

### Web Application Routing
Requests from Russian clients hit Yandex Cloud CDN. Origin: `app-prod-kiss2.getkisskiss.com` in the DigitalOcean Kubernetes cluster. The web application is deployed as a Deployment with `label app=app-web`.
Requests from non-Russian clients hit DigitalOcean App Platform with CDN enabled.

### API Routing
Requests from Russian clients hit Yandex Cloud Network Load Balancer, then reach the Nginx Ingress controller in the managed Kubernetes cluster. These requests are then proxied to `api-prod-fra1.kisskissplay.com` located in the DigitalOcean Kubernetes cluster via its Nginx Ingress controller.
Requests from non-Russian clients hit the DigitalOcean Kubernetes load balancer, then reach the Nginx Ingress controller in the managed Kubernetes cluster.

### WebSocket and WebSocket API
Requests from Russian clients hit Yandex Cloud Network Load Balancer, then reach the Nginx Ingress controller in the managed Kubernetes cluster, and finally arrive at Centrifugo running as a Deployment in the K8s cluster.
Requests from non-Russian clients hit the DigitalOcean Kubernetes load balancer, then reach the Nginx Ingress controller in the managed Kubernetes cluster, and finally arrive at Centrifugo running as a Deployment in the K8s cluster.

### Static Storage
Requests from Russian clients hit Yandex Cloud CDN. Origin: a corresponding Yandex Object Storage bucket.
Requests from non-Russian clients hit DigitalOcean CDN. Origin: a corresponding DigitalOcean Spaces bucket.

## Network Layer
All servers reside within a private cloud network. External access is restricted. Servers can only communicate with each other internally. Developers connect via VPN.

## Application Layer

### Frontend
Written in Flutter.
Deployed via a custom Helm chart `frontendapp`: https://github.com/playneta/helm-charts/tree/main/frontenddapp into the DigitalOcean Kubernetes cluster. Namespace: `kiss`. Serves as the origin for Yandex CDN.
For non-Russian players, it is served through DigitalOcean App Platform with CDN.

### API Backend
The backend follows a microservices architecture, written in Go. Each backend service is a GitHub monorepo named `go-kiss2-<service-name>`.
Backends are deployed in DigitalOcean managed Kubernetes.
Namespace: `kiss`
Labels:
* `app.kubernetes.io/name=<service-name>-<component-type>`
* `app.kubernetes.io/part-of=<service-name>`

Main component types:
* **api** — handles API requests at `api-prod.kisskissplay.com/<service-name>/`
* **consumer** — reads events from Redpanda
* **job** — runs as a Kubernetes CronJob
* **worker** — background processing

Pod environment variables define connectivity configuration. These reveal which data stores or adjacent services a backend connects to. For example:
```
- name: DB_CONNECT_URL        # PostgreSQL connection
  value:
- name: SCYLLA_CONNECT_URL    # ScyllaDB connection
  value:
- name: KAFKA_SEED_BROKERS    # Redpanda connection
  value:
- name: REDIS_SENTINEL_URL    # Valkey connection
  value:
- name: OPENSEARCH_HOST       # OpenSearch connection
  value:
```
Environment variables are populated from HashiCorp Vault via BanzaiCloud Vault Webhook.
Backends are deployed using a shared Helm chart `backendapp`: https://github.com/playneta/helm-charts/tree/main/backendapp

## Data Layer
PostgreSQL connections are routed through PgCat, running as a Deployment in the Kubernetes cluster.

Data store types:
* **PostgreSQL** — dedicated host per backend service
* **ScyllaDB** — shared cluster
* **Redpanda** — shared cluster
* **Valkey** — Valkey Sentinel installation; data persistence is critical
* **OpenSearch** — stores chat messages
* **Object Storage** — S3-compatible storage (DigitalOcean Spaces, Yandex Object Storage)

All data stores are deployed on DigitalOcean Droplets.
Naming convention: `<project>-<env>-<region>-<store-type>-<service-name>-<index>.getkisskiss.com`
Example: `kiss2-prod-fra1-postgres-bottle-0.getkisskiss.com`
When a data store serves multiple backend services, the service name is omitted:
`kiss2-prod-fra1-scylladb-0.getkisskiss.com`

Droplet tags:
* `project`: `kiss2`
* `env`: `dev/stage` or `prod`
* `purpose`: e.g. `postgres`, `redpanda`
* `region`: e.g. `fra1`

## Infrastructure as Code
Most infrastructure service definitions reside in: https://github.com/playneta/kiss2-infra
* Cloud infrastructure is managed with **Terragrunt** — `terraform/` directory
* Linux system configuration on Droplets is managed with **Ansible** — `ansible/` directory
* Workload deployment is managed with **Helmfile** — `helm/` directory

Terraform modules: https://github.com/playneta/terraform-modules
Helm charts: https://github.com/playneta/helm-charts
Ansible roles: https://github.com/playneta/ansible-roles/

## Observability
* Metrics: `kube-prometheus-stack` (Prometheus + Grafana)
* Logs: `Grafana Loki`
* Traces and errors: `Sentry`
* Alerts are dispatched via `Alertmanager`
* Monitoring stack namespace: `monitoring`
* Logging stack namespace: `logging`
* IaC repository: https://github.com/playneta/devops_observability

## CI/CD
Continuous integration and delivery are implemented with GitHub Actions.
Reusable workflow templates are stored in: https://github.com/playneta/devops-github-workflows
Self-hosted GitHub runners are deployed in the Kubernetes cluster, namespace: `arc-systems`

---

<!-- SUGGESTED ADDITIONS FOR A COMPREHENSIVE KNOWLEDGE BASE -->

## Secrets Management
* All application secrets are stored in **HashiCorp Vault**
* Secrets are injected into pods at runtime via **BanzaiCloud Vault Webhook** (mutating admission webhook)
* Vault access policies, authentication methods, and secret paths should be documented here
<!-- TODO: Add Vault cluster address, auth methods (Kubernetes auth, AppRole), secret engine paths, rotation policies -->

## Security
* Network policies and firewall rules governing inter-service communication
* Cloud firewall rules on DigitalOcean and Yandex Cloud
* Container image scanning pipeline and policies
* RBAC policies in Kubernetes (roles, bindings, service accounts)
* VPN solution used for developer access (type, provider, configuration)
* TLS certificate management (cert-manager, Let's Encrypt, or manual)
<!-- TODO: Document specific security controls, compliance requirements, and incident response procedures -->

## Disaster Recovery and Backup
* Database backup schedules and retention policies (PostgreSQL, ScyllaDB, Valkey, OpenSearch)
* Backup storage locations and encryption
* Recovery Time Objective (RTO) and Recovery Point Objective (RPO) targets
* Tested restore procedures and runbooks
* Cross-region replication strategy (if any)
<!-- TODO: Document backup tooling (pgBackRest, snapshots, etc.), tested recovery procedures -->

## Scaling and Capacity
* Horizontal Pod Autoscaler (HPA) policies for backend services
* Kubernetes cluster autoscaling configuration (node pools, min/max nodes)
* Droplet sizing for data stores (CPU, RAM, disk per store type)
* Current resource quotas and limits per namespace
* Expected traffic patterns and peak load windows
<!-- TODO: Add specific HPA thresholds, node pool configurations, Droplet sizes -->

## Service Catalog
* Complete list of backend microservices with their repositories, owners, and dependencies
* Service dependency graph (which service talks to which data store / other service)
* API versioning strategy and documentation links
* Health check and readiness probe endpoints per service
<!-- TODO: Enumerate all go-kiss2-* services, their component types, and inter-service dependencies -->

## Incident Management
* On-call rotation and escalation policies
* Alert routing rules (which alerts go to which channels/teams)
* Communication channels (Mattermost, Slack, PagerDuty)
* Post-incident review process
* Known incident runbooks (e.g., database failover, Redpanda partition reassignment)
<!-- TODO: Add specific alert routes, runbook links, escalation contacts -->

## Cost Management
* Cloud provider billing breakdown (DigitalOcean, Yandex Cloud, Gcore)
* Resource tagging strategy for cost attribution
* Reserved instances or committed use discounts
* Cost optimization initiatives and targets
<!-- TODO: Add monthly cost baseline, biggest cost drivers, optimization opportunities -->

## Environment Differences
* Differences between `dev/stage` and `prod` environments (resource sizes, replica counts, feature flags)
* Data masking or anonymization in non-production environments
* Environment promotion workflow (stage -> prod)
<!-- TODO: Document specific differences in cluster sizes, data store configs, and feature toggles -->
