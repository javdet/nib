# Knowledge Base

<!--
This is the default template. It is not indexed and not stored until you upload
it: fill in what applies, delete what does not, and upload it on the Knowledge
Base page. Uploading replaces every chunk in the collection.

How to write for the agent:
- State facts, not prose. Retrieval returns ~500-character chunks by semantic
  similarity, so every section must make sense on its own, without the ones
  around it.
- Spell names out in full at least once — cluster names, namespaces, hostnames,
  repository URLs, tags. The agent matches on them literally.
- Prefer patterns with a worked example over abstract descriptions.
- Never put secret values here. Record where a secret lives, not what it is.
-->

## Project Overview
What this project is, what it delivers, and who owns it. Business criticality
and traffic profile. Owning team, on-call rotation, links to the wiki space and
issue tracker board.

## Environments
Every environment (dev / stage / prod): its purpose, who may deploy to it, how
it differs from production, change-freeze windows, and the promotion path
between them.

## Clouds, Accounts and Regions
Cloud providers in use and the account / project / subscription identifiers.
Regions and availability zones per environment, why each was chosen, and any
quota or data-residency limits.

## Naming and Tagging Conventions
The naming pattern for hosts, DNS records, buckets, clusters and workloads, with
a worked example. Mandatory tags and labels, and what depends on them (cost
attribution, automation, alert routing).

## Repositories
Infrastructure and application repositories with their URLs and what lives in
each. Directory layout of the IaC repository. Branch model, review requirements,
and who can merge.

## Traffic Routing
Primary public endpoints and what serves each.

### DNS
Zones, DNS provider, who manages records, and any geo- or latency-based routing.

### Web Application Routing
Path from client to frontend: CDN, origin, load balancers, ingress.

### API Routing
Path from client to API, including any cross-region proxying.

### WebSocket and WebSocket API
Realtime endpoints and the components behind them.

### Static Storage
Public and private object storage endpoints, buckets and their CDN origins.

## Network Layer
Private networks and CIDR ranges, subnet layout, peering and VPN links, firewall
and security-group rules that govern inter-service traffic, and how engineers
reach private resources (VPN, bastion).

## Compute and Orchestration
Kubernetes clusters — names, versions, node pools, sizes and autoscaling bounds.
Namespaces and what runs in each. Standalone VMs and what they host. How
workloads are deployed (Helm charts and their sources, Helmfile, Argo CD).

## Application Layer

### Frontend
Language and framework, build output, how and where it is deployed, and the
chart or platform serving it.

### API Backend
Architecture (monolith / microservices), language and runtime, repository
naming, component types (api, worker, consumer, job) and what each does.
Deployment mechanism and workload labels. The environment-variable contract
that reveals which data stores a service talks to.

## Data Layer
Each store type (relational, wide-column, cache, search, queue, object storage):
which services use it, topology (dedicated vs shared, replication), host naming,
connection routing through poolers or proxies, and how schema migrations are
applied.

## Infrastructure as Code
Tools in use (Terraform / Terragrunt, Ansible, Helmfile) and the directory each
owns. Where remote state and locking live. The exact commands to plan and apply,
and what requires approval. Shared module and role registries. Anything managed
outside IaC, and the known drift.

## CI/CD
Pipeline system and where reusable workflow definitions live. Runner topology
(hosted vs self-hosted, and the namespace for self-hosted). Image registry and
tagging scheme. Release and promotion flow, required approvals, and the rollback
procedure.

## Secrets Management
Secret store and how secrets reach workloads (injector, CSI driver, sealed
secrets). Authentication methods, path conventions, rotation policy, and who may
read what. Values never belong in this document.

## Access and Permissions
Cloud IAM and Kubernetes RBAC model. Service accounts and what they can do. How
an engineer — or an automated agent — obtains credentials for each environment.
Break-glass procedure.

## Observability
Metrics, logs, traces and error-tracking stacks, their namespaces and URLs. Key
dashboards and what each answers. SLOs and error budgets. Alert routing: which
alert reaches which channel or team.

## Incident Management
Severity definitions, on-call rotation and escalation path. Communication
channels. Index of known runbooks (failover, partition reassignment, cache
flush). Post-incident review process.

## Backup and Disaster Recovery
What is backed up, on what schedule, with what retention, and where the backups
are stored. RPO and RTO targets per store. Tested restore procedures and when a
restore was last exercised. Cross-region replication, if any.

