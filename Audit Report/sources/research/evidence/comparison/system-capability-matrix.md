# Version-specific system capability matrix (as of 2026-09-09)

## Scope and rating rule

This matrix compares only the eight requested criteria. **D** means the pinned,
version-specific primary source directly documents the capability. **P** means
the capability is documented only as a narrower, profile-specific implementation
property. **NE** means the reviewed source did not establish the capability; it is
not evidence that the capability is absent.

## Matrix

| System/version | Criterion | Rating | Exact source | Evidence and interpretation/limitation |
|---|---|---:|---|---|
| AutoMCP, arXiv:2507.16044v4 | 1. API-derived tools | D | [paper v4, Sec. 3.3](https://arxiv.org/html/2507.16044v4#S3.SS3) | The paper describes AutoMCP as a compiler that converts OpenAPI 2.0/3.0 endpoints into callable MCP tools and states that each endpoint is compiled into a tool. |
| AutoMCP, arXiv:2507.16044v4 | 2. Selective exposure | D | [paper v4, Sec. 3.5](https://arxiv.org/html/2507.16044v4#S3.SS5) | The extended pipeline applies category filtering and Collection/Item grouping. This establishes automated tool-set transformation, not an operator approval UI. |
| AutoMCP, arXiv:2507.16044v4 | 3. Explicit revision targeting | NE | [paper v4](https://arxiv.org/html/2507.16044v4) | The reviewed paper discusses generated servers, filtering, grouping, and authentication, but no client-requested tool revision or revision identifier. |
| AutoMCP, arXiv:2507.16044v4 | 4. Approval-bound exact contract-and-mapping resource | NE | [paper v4, Sec. 3.3](https://arxiv.org/html/2507.16044v4#S3.SS3) | The source describes generated code and specification repair. It does not establish an approval record bound to an immutable contract-and-mapping resource. |
| AutoMCP, arXiv:2507.16044v4 | 5. Persisted desired-state reconciliation | NE | [paper v4, Sec. 3.3--3.5](https://arxiv.org/html/2507.16044v4#S3.SS3) | AutoMCP emits a runnable server and transformation output; no persisted desired/observed control state or reconciliation controller is documented. |
| AutoMCP, arXiv:2507.16044v4 | 6. Per-call revision consistency | NE | [paper v4, Sec. 3.3](https://arxiv.org/html/2507.16044v4#S3.SS3) | The source documents generated handlers, schema validation, and authentication injection, but not a revision identity carried through validation, policy, and execution. |
| AutoMCP, arXiv:2507.16044v4 | 7. MCP client authentication | NE | [paper v4, Sec. 3.3](https://arxiv.org/html/2507.16044v4#S3.SS3) | Authentication handling in the paper concerns the generated server's access to the vendor API. It does not directly document authentication of MCP clients to that generated server. |
| AutoMCP, arXiv:2507.16044v4 | 8. Downstream/upstream API credential mediation outside model arguments | D | [paper v4, Sec. 3.3](https://arxiv.org/html/2507.16044v4#S3.SS3) | AutoMCP maps declared API security schemes to environment variables and generates a `.env` template; credentials are supplied separately from tool arguments. The paper still leaves the only required credential-provisioning step manual. |
| FastMCP 4.0.3 | 1. API-derived tools | D | [pinned OpenAPI integration docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/integrations/openapi.mdx#L10-L38) | `FastMCP.from_openapi()` converts an OpenAPI specification into MCP components. The docs state that, by default, every endpoint becomes an MCP Tool. |
| FastMCP 4.0.3 | 2. Selective exposure | D | [pinned OpenAPI route maps](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/integrations/openapi.mdx#L40-L120); [visibility](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/visibility.mdx#L1-L120) | Route maps can assign `EXCLUDE`, and the server visibility API supports names, tags, versions, keys, and allow-list mode. This is framework-level filtering, not approval-bound governance. |
| FastMCP 4.0.3 | 3. Explicit revision targeting | D | [pinned versioning docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/versioning.mdx#L125-L205) | FastMCP documents versioned components, exact `call_tool(..., version="1.0")`, and the generic-client `_meta.fastmcp.version` request convention. |
| FastMCP 4.0.3 | 4. Approval-bound exact contract-and-mapping resource | NE | [pinned tools docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/tools.mdx) | The reviewed framework docs describe registration, schemas, metadata, and versioning, but no approval record binding an exact API contract and execution mapping. |
| FastMCP 4.0.3 | 5. Persisted desired-state reconciliation | NE | [pinned server docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/server.mdx) | The source documents in-process servers, providers, transforms, and visibility. It does not establish a persisted desired-state controller that repairs runtime exposure. |
| FastMCP 4.0.3 | 6. Per-call revision consistency | NE | [pinned versioning docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/versioning.mdx#L170-L205) | Exact version selection is documented, but the reviewed source does not state that authorization/policy and execution all use one immutable per-call revision snapshot. |
| FastMCP 4.0.3 | 7. MCP client authentication | D | [pinned authentication docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/servers/auth/authentication.mdx#L1-L100) | FastMCP documents HTTP/SSE MCP authentication, token verification, OAuth providers, and external identity providers. It notes that stdio inherits local execution security. |
| FastMCP 4.0.3 | 8. Downstream/upstream API credential mediation outside model arguments | D | [pinned OpenAPI authentication docs](https://github.com/PrefectHQ/fastmcp/blob/v4.0.3/docs/integrations/openapi.mdx#L26-L38) | The OpenAPI integration configures an authenticated HTTP client with an Authorization header before constructing the MCP server. The example uses a placeholder token in client configuration, not a model argument. |
| IBM MCP Context Forge 1.0.9 | 1. API-derived tools | D | [pinned README, REST-to-MCP adapter](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/README.md#rest-to-mcp-tool-adapter); [OpenAPI schema service](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/services/openapi_service.py) | The release README says ContextForge adapts REST APIs into tools with automatic JSON Schema extraction, headers, tokens, auth, retries, and rate limits. The pinned service extracts request/response schemas from OpenAPI. |
| IBM MCP Context Forge 1.0.9 | 2. Selective exposure | D | [pinned README virtual-server workflow](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/README.md#end-to-end-demo-register-a-local-mcp-server) | The documented workflow lists tools, then creates a virtual server with an explicit `associated_tools` list. This establishes selection of catalog tools for a virtual server, not immutable approval. |
| IBM MCP Context Forge 1.0.9 | 3. Explicit revision targeting | NE | [release v1.0.9 notes](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/CHANGELOG.md#109---2026-08-31---mtls-oauth-quick-wins-tool-preview-catalog-actions-and-security-hardening) | The release notes mention versioned API aliases, but do not document client selection of a particular MCP tool revision. API route aliases are not treated as tool-revision targeting. |
| IBM MCP Context Forge 1.0.9 | 4. Approval-bound exact contract-and-mapping resource | NE | [pinned tool service](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/services/tool_service.py) | The service persists tool definitions, schemas, mappings, and encrypted auth material, but the reviewed source does not establish an approval record binding an exact contract-and-mapping revision. |
| IBM MCP Context Forge 1.0.9 | 5. Persisted desired-state reconciliation | NE | [pinned gateway service](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/services/gateway_service.py) | ContextForge persists gateway/tool records and performs health/federation management. The reviewed release source does not establish a ToolPlane-style persisted desired-versus-observed reconciliation loop for tool exposure. |
| IBM MCP Context Forge 1.0.9 | 6. Per-call revision consistency | NE | [pinned tool service](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/services/tool_service.py) | The service documents resolution, RBAC, visibility, validation, and invocation paths, but no single immutable revision identity is shown to bind validation, policy, and execution for each call. |
| IBM MCP Context Forge 1.0.9 | 7. MCP client authentication | D | [pinned README authentication](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/README.md#quick-start---pypi); [pinned auth module](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/auth.py) | The release requires JWT and encryption secrets and documents bearer-token generation and authenticated API access. The pinned auth module implements token/session/team resolution and revocation checks. |
| IBM MCP Context Forge 1.0.9 | 8. Downstream/upstream API credential mediation outside model arguments | D | [pinned README feature list](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/README.md#overview--goals); [pinned tool service](https://github.com/IBM/mcp-context-forge/blob/v1.0.9/mcpgateway/services/tool_service.py) | The release README documents user-scoped OAuth tokens and `X-Upstream-Authorization`; the tool service encrypts sensitive auth headers for storage and decrypts them for runtime requests. |
| ERPBridge/ToolPlane reference prototype | 1. API-derived tools | P | [`research/project/ToolPlane-Reference-Implementation-Architecture.md`, API-to-MCP flow, lines 9--17](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/idp/generator.go`, `GenerateFromOpenAPI`](../../../../ERPBridge/internal/idp/generator.go) | The prototype registers an ERP API, generates one declarative MCP tool per OpenAPI operation, and preserves typed parameters and mappings. This is a profile-specific implementation fact, not a general ToolPlane outcome. |
| ERPBridge/ToolPlane reference prototype | 2. Selective exposure | P | [`ToolPlane-Reference-Implementation-Architecture.md`, lines 17--23](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/mcp/server.go`, `handleToolApply`/`handleToolList`](../../../../ERPBridge/internal/mcp/server.go) | Only applied, validated, active resources are exposed and the operator can choose which generated manifests to apply. The reviewed prototype does not establish a general policy language for arbitrary client-specific views. |
| ERPBridge/ToolPlane reference prototype | 3. Explicit revision targeting | P | [`../ERPBridge/internal/mcp/registry.go`, `ParseToolIdentifier` and `Resolve`](../../../../ERPBridge/internal/mcp/registry.go); [`../ERPBridge/internal/mcp/server.go`, qualified-call resolution](../../../../ERPBridge/internal/mcp/server.go) | The prototype supports qualified names and, in the evaluated boundary profile, digest-bound calls. The syntax and behavior are implementation-specific and are not presented as a generic MCP protocol feature. |
| ERPBridge/ToolPlane reference prototype | 4. Approval-bound exact contract-and-mapping resource | P | [`../ERPBridge/internal/mcp/tool.go`, `Tool.Metadata`/`ToolSpec`]; [`../ERPBridge/internal/mcp/store.go`, admission digest persistence](../../../../ERPBridge/internal/mcp/store.go); [`ToolPlane-Reference-Implementation-Architecture.md`, lines 15--22](../../project/ToolPlane-Reference-Implementation-Architecture.md) | The prototype stores declarative MCPTool resources, records admission metadata/digests in the evaluated branch, and rejects admitted-content mutation. This is a reference-prototype profile, not a claim about all ToolPlane deployments. |
| ERPBridge/ToolPlane reference prototype | 5. Persisted desired-state reconciliation | P | [`ToolPlane-Reference-Implementation-Architecture.md`, lines 21--24](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/mcp/server.go`, `Reconcile`/`reconcileLocked`](../../../../ERPBridge/internal/mcp/server.go); [`../ERPBridge/internal/mcp/store.go`, reconciliation status/tombstones](../../../../ERPBridge/internal/mcp/store.go) | SQLite stores desired tools and the controller restores active registrations, serving state, and plugin bindings. The documented bound is six attempts with delays 0/250/500/1000/2000/3000 ms under a 10-second deadline. |
| ERPBridge/ToolPlane reference prototype | 6. Per-call revision consistency | P | [`ToolPlane-Reference-Implementation-Architecture.md`, Tool invocation section](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/mcp/server.go`, snapshot and final active-resource verification](../../../../ERPBridge/internal/mcp/server.go); [`../ERPBridge/internal/mcp/plugin_pipeline.go`, `executeToolCall`](../../../../ERPBridge/internal/mcp/plugin_pipeline.go) | The evaluated branch resolves a revision, snapshots it, applies policy, checks active authority immediately before dispatch, and then invokes the selected revision. The guarantee is scoped to the single-process prototype. |
| ERPBridge/ToolPlane reference prototype | 7. MCP client authentication | P | [`ToolPlane-Reference-Implementation-Architecture.md`, deployment architecture](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/mcp/server.go`, `AuthHandler` mounting](../../../../ERPBridge/internal/mcp/server.go); [`../ERPBridge/docs/tokens.md`](../../../../ERPBridge/docs/tokens.md) | Streamable HTTP uses authenticated caller identity for guarded tools; stdio has no HTTP caller identity and guarded tools fail closed. This is an implementation profile, and it does not claim MCP 2026-07-28 conformance. |
| ERPBridge/ToolPlane reference prototype | 8. Downstream/upstream API credential mediation outside model arguments | P | [`ToolPlane-Reference-Implementation-Architecture.md`, Tool invocation, lines 33--39](../../project/ToolPlane-Reference-Implementation-Architecture.md); [`../ERPBridge/internal/mcp/tool.go`, `resolveCredential`/`prepareERPCall`](../../../../ERPBridge/internal/mcp/tool.go); [`../ERPBridge/internal/credentials`](../../../../ERPBridge/internal/credentials) | `credentialRef` points to environment/file-backed credentials and the runtime injects them into ERP requests. Secrets are therefore kept outside MCP arguments and manifests, subject to the prototype's credential-source constraints. |

## Source handling and gaps

### Kept

* AutoMCP arXiv HTML v4 is the paper's full primary text, not an abstract or
  secondary summary.
* FastMCP documentation and source paths are pinned to the `v4.0.3` tag.
* Context Forge documentation, release notes, and source paths are pinned to
  the `v1.0.9` tag.
* ToolPlane evidence uses the repository architecture baseline and ERPBridge
  source paths. The local evaluated ERPBridge branch is the authoritative source
  for prototype-only revision/admission/reconciliation behavior.

### Excluded or not used as capability evidence

* Search-result summaries, release aggregators, PyPI metadata, and current
  `main` documentation were not used to assign D ratings when a pinned source
  was available.
* A release note saying “versioned API aliases” was not interpreted as explicit
  MCP tool revision targeting.
* Absence of a term in a source was recorded as NE, never as proof that a
  system lacks the capability.

### Gaps requiring care in Table I

1. FastMCP's versioning and visibility are directly documented, but approval
   binding, persisted desired-state reconciliation, and per-call policy/runtime
   identity are not established by the reviewed v4.0.3 sources.
2. Context Forge has persistence, RBAC, federation, and versioned API aliases,
   but the reviewed v1.0.9 sources do not establish the narrower ToolPlane
   semantics of approved immutable revisions and desired/observed reconciliation.
3. ERPBridge's advanced admission and revision behavior is available in the
   local evaluated checkout and project baseline, not as a public immutable
   GitHub commit. It should therefore remain marked P and be cited as a
   prototype implementation fact.
4. Authentication ratings distinguish MCP client authentication from downstream
   ERP/API credential mediation. OpenAPI security handling alone does not imply
   MCP client authentication.
