# Plan: Deterministic Mock ERP Integration Fixtures

## Goal

Provide a small, authenticated, deterministic Mock ERP fixture surface for
black-box integration tests. The fixture must support both the SDK/ERPBridge
registry integration plan and the external-plugin integration plan without
mixing test-only ERP data into either feature implementation.

The fixture is not a production ERP simulation. It returns stable, non-PII
JSON and exposes enough readback to prove what ERPBridge sent downstream.

## Current State

- The FastAPI application registers finance, HR, and inventory routers plus a
  health route (`mock-erp/main.py:1-25`). It has no integration-specific router.
- Inventory currently owns static resource data and authenticated resource
  handlers (`mock-erp/routers/inventory.py:1-6,105-133`). There is no
  deterministic plugin fixture or echo endpoint.
- The existing dependency layer requires a valid ERPNext-style credential and
  resolves it to a mock role (`mock-erp/dependencies.py:7-42`). New fixtures
  can therefore exercise the same downstream authentication boundary.
- The OpenAPI document describes the current resource paths through
  `/resource/Purchase Invoice` through `/resource/Purchase Order`
  (`mock-erp/openapi.yaml:161-438`) but does not describe the integration
  fixtures.
- The SDK integration plan needs a non-mutating echo endpoint that records the
  last request. The external-plugin plan needs a stable `Plugin Fixture`
  resource. Both plans currently list Mock ERP source changes in their own
  tasks instead of sharing one fixture contract.

## Decisions

1. **One owner for Mock ERP fixture changes.** This plan owns the Mock ERP
   routes, OpenAPI description, fixture tests, and fixture documentation. The
   SDK integration and external-plugin plans consume these contracts and do
   not edit `mock-erp/*`.
2. **Use exact stable endpoints.** Provide:
   - `GET /api/resource/Plugin Fixture`, returning exactly
     `{"data":{"id":"plugin-fixture","state":"source"}}`.
   - `POST /api/integration/echo`, returning a `data` property equal to the
     received JSON object and recording that object in process memory.
   - `GET /api/integration/echo/last`, returning a `data` property containing
     the last received JSON object, or `null`, for test inspection.
3. **Require downstream authentication.** All three fixtures use the existing
   `get_role` dependency. A valid mock ERP credential is supplied by the
   isolated integration environment; invalid or missing credentials return the
   existing ERPNext-style `401` response.
4. **Do not mutate or enrich payloads.** The echo route must return the JSON
   object it received and must not add a `role`, credential, timestamp, or
   request identifier. The `Plugin Fixture` response must remain static.
5. **Keep the fixture process-local.** The last echo payload is test
   observation state only. It is not persisted, shared between processes, or
   treated as ERP data. Tests must not depend on request ordering across test
   processes.
6. **Keep the contract intentionally narrow.** Reject non-object echo payloads
   with a deterministic `422` validation response. Do not add CRUD, filtering,
   pagination, PII, binary data, or plugin behavior to the Mock ERP.

## Scope

### In scope

- A dedicated Mock ERP integration router.
- The static `Plugin Fixture` resource and echo/readback endpoints.
- FastAPI behavior tests, dependency-group test tooling, and lockfile updates.
- OpenAPI paths/schemas/security declarations and Mock ERP README examples.
- Handoff references from the SDK integration and external-plugin plans.

### Out of scope

- ERPBridge server, SDK, workflow-engine, or external-plugin implementation.
- A separate mock-plugin process or Docker Compose integration overlay.
- Production ERP behavior, persistence, background jobs, or dynamic fixture data.
- Changes to existing finance, HR, inventory, or authentication semantics.

## Tasks

- [ ] **Task 1: Establish red tests and the isolated Python test dependencies.**
  Add a `test` dependency group containing `pytest>=8.3.5`,
  `httpx>=0.28.1`, and `pyyaml>=6.0.2`, then update `uv.lock`. Add behavior
  tests before the routes exist. Use FastAPI's in-process client
  with the valid mock admin credential and assert the exact success envelopes,
  missing/invalid credential failures, echo readback, payload preservation, and
  non-object rejection. Add a test that confirms the `role` key is not added to
  an echoed payload. Add a YAML contract test for the three documented paths,
  their response shapes, and the `TokenAuth` security requirement.

  **Seam:** FastAPI application boundary (`app`) and the checked-in OpenAPI
  contract.

  **Files:**
  `mock-erp/tests/test_integration_fixtures.py` (new),
  `mock-erp/pyproject.toml`, `mock-erp/uv.lock`.

  **Verify:**

  ```bash
  cd mock-erp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  ```

  The first run must be red because the fixture routes are not implemented.

- [ ] **Task 2: Implement the authenticated deterministic fixture routes.**
  Add a dedicated router and register it from the FastAPI application. Keep
  the resource fixture separate from inventory data. Implement the exact
  response envelopes and in-memory last-payload readback from the Decisions
  section. Use `Depends(get_role)` on every route and validate that the echo
  request body is a JSON object before recording it.

  **Seam:** HTTP request → `get_role` → integration router → JSON response.

  **Files:**
  `mock-erp/routers/integration.py` (new), `mock-erp/main.py`,
  `mock-erp/tests/test_integration_fixtures.py`.

  **Verify:**

  ```bash
  cd mock-erp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  uv run python -m compileall -q main.py routers tests
  ```

- [ ] **Task 3: Publish the fixture contract in OpenAPI and the Mock ERP guide.**
  Add schemas for `PluginFixture`, `IntegrationEchoRequest`, and the response
  envelopes. Add the three paths with the existing `TokenAuth` security scheme,
  exact request/response examples, and `422`/`401` responses where applicable.
  Document the endpoints, deterministic payloads, valid test credential
  convention, and the process-local readback limitation in `mock-erp/README.md`.

  **Seam:** checked-in OpenAPI contract → tool-generation consumers and human
  integration-test operators.

  **Files:**
  `mock-erp/openapi.yaml`, `mock-erp/README.md`,
  `mock-erp/tests/test_integration_fixtures.py`.

  **Verify:**

  ```bash
  cd mock-erp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  uv run python -c 'import yaml; yaml.safe_load(open("openapi.yaml", encoding="utf-8")); print("valid OpenAPI YAML")'
  ```

- [ ] **Task 4: Repoint the consuming plans to this shared fixture contract.**
  Remove Mock ERP implementation files from the SDK integration plan's fixture
  task and make it consume `/api/integration/echo`. Remove Mock ERP
  implementation files from the external-plugin plan's black-box task and make
  it consume `/api/resource/Plugin Fixture`. Add this plan as the prerequisite
  for both tasks. Update the ERPBridge plan index and upcoming-plan index so
  this plan is discoverable and remains non-executable until promoted.

  **Seam:** plan-to-plan fixture contract and task ownership, not runtime code.

  **Files:**
  `.agents/plans/active/Plan-SDK-Integration-Testing.md`,
  `.agents/plans/upcoming/Plan-Generic-External-Plugins.md`,
  `.agents/plans/README.md`, `.agents/plans/upcoming/README.md`.

  **Verify:**

  ```bash
  rg -n 'Plan-Mock-ERP-Fixtures|/api/integration/echo|Plugin Fixture' \
    .agents/plans/active/Plan-SDK-Integration-Testing.md \
    .agents/plans/upcoming/Plan-Generic-External-Plugins.md \
    .agents/plans/README.md .agents/plans/upcoming/README.md
  if rg -n 'mock-erp/(routers|main.py|openapi.yaml|README.md)' \
    .agents/plans/active/Plan-SDK-Integration-Testing.md \
    .agents/plans/upcoming/Plan-Generic-External-Plugins.md; then
    echo 'consumer plans still own Mock ERP files' >&2
    exit 1
  fi
  ```

  The first command must show the shared dependency and endpoint contracts;
  the second must show no Mock ERP implementation file ownership in either
  consumer task.

## Verification

The plan is complete when:

- The `Plugin Fixture` endpoint returns the exact stable non-PII envelope.
- The echo endpoint returns and records the exact received JSON object.
- Missing or invalid ERPNext-style credentials fail with the existing `401`
  response for every fixture route.
- Echo payloads do not gain `role` or other authorization/request metadata.
- Non-object echo payloads fail deterministically with `422` and are not
  recorded as successful requests.
- The OpenAPI document parses and describes all three routes with matching
  security and response contracts.
- Mock ERP tests and compilation pass without changing existing module routes.
- The SDK integration and external-plugin plans have one clear Mock ERP owner
  and consume this plan's endpoints without duplicating fixture work.

## Open Questions

None. The endpoint names, payloads, authentication boundary, and ownership
split are fixed for the shared integration fixture.
