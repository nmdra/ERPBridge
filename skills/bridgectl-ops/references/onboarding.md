# Onboard an ERP API as an MCP tool

## Outcome

An ERP endpoint is registered locally, connectivity is demonstrated, its tool
schema is reviewed and validated, and the approved manifest is visible through
the selected ERPBridge context.

## Workflow

1. Establish the context and authorization described in `SKILL.md`. Confirm
   the endpoint owner, URL, method, module, purpose, expected inputs, and the
   secret-manager reference for the ERP credential.
2. Inspect the current CLI help and in-repo `docs/cli/` pages for the installed
   version. Register the API only after confirmation. Source a required test
   credential from the environment; never paste it into a manifest or report.
3. Run `bridgectl api test <name>`. Stop on failure and follow the diagnosis
   workflow rather than publishing an unverified endpoint.
4. Generate a tool manifest with `bridgectl tool generate --api <name>`; add
   `--openapi <path-or-url>` when an OpenAPI document should produce multiple
   operations. Treat generated output as a draft.
5. Complete the intent metadata, input schema, execution path, `credentialRef`,
   cache policy, and optional `allowedRoles`. Use `assets/mcp-tool.yaml` from
   the skill root only as a field guide.
   A write operation normally disables cache and lists affected read tools in
   `flushOn` when cache invalidation is needed.
6. Run `bridgectl tool validate -f <manifest>`. Correct validation failures
   before applying. Confirm the target context and manifest before
   `bridgectl tool apply -f <manifest>`.
7. Verify with `bridgectl tool get <name>`, `bridgectl tool describe <name>`,
   and an MCP discovery/call that has safe test data. For guarded tools, verify
   both an authorized role and a denied role without exposing credentials.
8. If the tool needs post-response processing, read [Plugins](plugins.md).
   Apply and verify the exact plugin version first, then apply a binding to this
   exact tool version. Confirm the transformed result and output-schema behavior
   with safe data; keep plugin deployment outside this onboarding workflow.

## Completion evidence

Record the selected context, API test status, manifest version, validation
result, registry readback, and MCP verification. Do not include request bodies
or responses that contain secrets or personal data.
