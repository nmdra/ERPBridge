# Advisor Review: Minimal ERPBridge Demo Terraform

## Disposition

The original multi-root plan was reworked after review and user clarification.
The current target is one native-HCL Terraform root for a short-lived demo.
The user accepted sensitive values in Terraform state to remove the Key Vault
seeding stage and custom Python scripts.

The revised design is suitable for a fast experiment, with these conditions:

- Keep all real values in ignored `terraform.tfvars` or `TF_VAR_*` variables.
- Keep local state and plan files out of Git.
- Use immutable image digests.
- Use HTTPS ingress and the MockERP CIDR restriction.
- Destroy the deployment and delete state after the experiment.

## Required review points

1. `API_AUTH_TOKEN`, `ERP_PRIMARY_KEY`, and
   `MOCK_ERP_CREDENTIALS_JSON` are sensitive variables, but their values remain
   in Terraform state by design.
2. Terraform must configure direct Container App secrets with native HCL.
   Terraform must not output those values.
3. `API_AUTH_TOKEN` authenticates `bridgectl` to ERPBridge. `ERP_PRIMARY_KEY`
   authenticates ERPBridge to MockERP. They are different credentials.
4. The `bridgectl` `mcp-server` control-plane value must use the ERPBridge host
   root. The `/mcp/` suffix is reserved for MCP clients.
4. `deployment_summary`, `erpbridge_url`, `erpbridge_mcp_url`, and
   `mockerp_url` contain only non-sensitive deployment details.
5. ERPBridge ingress and probes use the same named MCP port. MockERP ingress
   and probes use fixed port 8081.
6. The first deployment leaves `ERP_BASE_URL` unset. Manual `bridgectl` API
   registration supplies the MockERP URL.

## Current execution status

- Private repository: created as `nmdra/erpbridge-infra`.
- Working branch: `feat/azure-container-apps-demo`.
- AzureRM: pinned to `5.2.0`.
- Terraform: one-root native HCL implementation completed.
- Static validation: Terraform format, initialization, and validation pass.
- Azure apply: completed for the short-lived demo.
- Git commit: `4ce9018`.
- Feature branch push: completed to
  `origin/feat/azure-container-apps-demo`.

## Remaining external inputs

- Azure subscription and tenant IDs.
- An immutable ERPBridge image digest.
- The MockERP `0.2.1` image digest.
- The local Postman IPv4 CIDR.
- Three short-lived secret values.
