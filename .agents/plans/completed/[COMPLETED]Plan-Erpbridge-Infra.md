# Plan: Minimal ERPBridge Demo Terraform

## Goal

Create a private `nmdra/erpbridge-infra` repository with one small Terraform
root. Deploy ERPBridge and MockERP as two Azure Container Apps in the existing
`erpbridge-hq` resource group. Expose both apps through HTTPS, use ephemeral
SQLite storage and one replica, omit Redis, and print safe connection details
for manual `bridgectl` use.

## Approved scope revision

This revision optimizes for a short-lived experiment and fast testing:

- Use one `terraform/` root instead of separate `foundation/` and `apps/`
  roots.
- Use local Terraform state by default. Do not bootstrap a remote state account.
- Use AzureRM provider `5.2.0`.
- Store the three demo secret values in Terraform state through sensitive
  variables. Do not commit state, plan files, `terraform.tfvars`, or values.
- Configure Container App secrets directly with native HCL. Do not use Key Vault,
  a custom Python seeder, or a custom image approval script.
- Keep image references immutable with `image@sha256:<64 hex>` validation.
- Keep `bridgectl` onboarding manual. Use Terraform outputs to construct the
  local URL configuration. Do not persist the bearer token in the context.

This state trade-off is acceptable because the deployment is short-lived.
Destroy the deployment and delete local state after the experiment.

## Decisions

1. Terraform reads `erpbridge-hq` with a data source. It does not create or
   delete the resource group. New resources use the explicitly selected
   policy-allowed `deployment_location`, which can differ from the resource
   group's metadata location.
2. Terraform creates one Log Analytics workspace and one Consumption Container
   Apps environment.
3. Terraform creates exactly two Container Apps:
   - `erpbridge-server`, using port 8080 by default.
   - `mockerp`, using fixed port 8081.
4. Both apps use external HTTPS ingress, one replica, and an `EmptyDir` volume.
5. MockERP uses a required IPv4 CIDR allow-list. Terraform rejects an empty
   list and `0.0.0.0/0`.
6. `ERP_BASE_URL` is omitted by default. The operator registers MockERP
   manually with `bridgectl` after deployment.
7. `API_AUTH_TOKEN`, `ERP_PRIMARY_KEY`, and `MOCK_ERP_CREDENTIALS_JSON` are
   sensitive Terraform variables. Their values configure Container App
   secrets and remain in Terraform state for this demo.
8. `API_AUTH_TOKEN` protects ERPBridge. Its hash drives the ERPBridge revision
   suffix, so a changed token starts a new revision. `ERP_PRIMARY_KEY`
   authenticates ERPBridge calls to MockERP. They are different values.
9. Terraform outputs only resource and connection details. It does not output
   secret values.

## Tasks

- [x] **Task 1: Create the private repository.** Create
  `nmdra/erpbridge-infra` on a non-main branch and add the repository README,
  `.gitignore`, and non-deploying Terraform CI workflow.
  **Verify:** `gh repo view nmdra/erpbridge-infra --json nameWithOwner,isPrivate`
  reports a private repository and the workflow does not apply Terraform.

- [x] **Task 2: Implement the native HCL deployment.** Add one Terraform root
  with the pinned AzureRM provider, existing resource-group lookup, Log
  Analytics workspace, ACA environment, both Container Apps, HTTPS ingress,
  probes, `EmptyDir` volumes, direct sensitive app secrets, typed runtime
  variables, and immutable image validation.
  **Verify:** `terraform fmt -check -recursive terraform`; `terraform init
  -backend=false`; `terraform validate`.

- [x] **Task 3: Add safe Terraform outputs.** Output a deployment summary and
  individual `erpbridge_url`, `erpbridge_mcp_url`, `mockerp_url`, resource
  group, and environment values. Do not output secret values.
  **Verify:** inspect `terraform/outputs.tf`; no sensitive variable is an
  output; `terraform output` after apply prints only safe details.

- [x] **Task 4: Document fast deployment and authentication.** Document shell
  variables, secret input, token changes, manual Postman checks, and the
  distinction between the ERPBridge bearer token and the ERPNext credential.
  Add `docs/bridgectl-configuration.md` showing how to construct URL settings
  from Terraform output without storing the bearer token.
  **Verify:** documentation contains no real credential, token, or secret.

- [x] **Task 5: Run the demo deployment and manual verification.** Supply a
  valid Azure subscription, tenant, policy-allowed deployment location,
  immutable ERPBridge and MockERP image digests, Postman CIDR, and three
  short-lived secret values. Run `terraform plan`, review it, run
  `terraform apply`, inspect the safe output, verify both HTTPS health
  endpoints, verify ERPBridge bearer authentication, and perform manual
  `bridgectl` registration.
  **Verify:** both apps run with one replica; MockERP is reachable only from
  the configured CIDR and requires its ERPNext credential; protected ERPBridge
  operations reject missing or old bearer tokens; no Redis resource exists.
  **Result:** centralindia was selected because the subscription policy
  allowed that region. HTTPS health and both authentication boundaries passed.
  A token change created a new ERPBridge revision; the old token returned 401
  and the new token returned 200. Manual registration passed in an isolated
  `bridgectl` context.

- [x] **Task 6: Review and publish the repository.** Run the final format,
  validation, shell, secret, and diff checks. Review the Terraform plan and
  commit the implementation on the feature branch. Push the feature branch
  only after review. Do not commit on `main`.
  **Verify:** the repository has no state, plan, secret, or credential files;
  CI passes; the feature branch contains the implementation.
  **Result:** commit `4ce9018` was pushed to
  `origin/feat/azure-container-apps-demo`. The local state remains ignored and
  is not committed.

## Verification commands

Run these commands from `/home/nimendra/Documents/Projects/erpbridge-infra`:

```bash
terraform fmt -check -recursive terraform
terraform -chdir=terraform init -backend=false -input=false
terraform -chdir=terraform validate -no-color
```

The deployment command is:

```bash
terraform -chdir=terraform plan
terraform -chdir=terraform apply
```

The deployment command must run only after the operator supplies real values in
an ignored `terraform.tfvars` file or through `TF_VAR_*` variables.

## Cleanup

```bash
terraform -chdir=terraform destroy
rm -f terraform/terraform.tfstate terraform/terraform.tfstate.backup
```

Do not delete `erpbridge-hq`.
