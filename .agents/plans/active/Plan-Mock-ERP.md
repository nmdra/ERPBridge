# Plan: ERPNext-Aligned Mock ERP for Serendib Consumer Products

## Goal

Evolve the authenticated Mock ERP from three static read-only fixture modules into
an independent, SQLite-backed, deterministic legacy ERP for **Serendib Consumer
Products (Pvt) Ltd (SCP)**: a fictional Sri Lankan FMCG manufacturer, importer,
and wholesale distributor. The service moves to the new public repository
`nmdra/mockerp`, preserving the Mock ERP subtree's Git history. ERPBridge
consumes the published, version-pinned image
`ghcr.io/nmdra/mockerp:0.2.1` and fetches its matching versioned OpenAPI contract;
it no longer builds or vendors the Mock ERP source tree.

The completed system will model SCP's Peliyagoda head office and warehouse,
Katunayake factory and raw-material warehouse, and Kandy and Galle distribution
centres. It uses LKR and fictional, non-PII seed data for household and
industrial cleaning products.

## Current State

- Mock ERP is a small FastAPI application that registers only finance, HR, and
  inventory routers (`mock-erp/main.py:1-25`). Its current resource data is
  process-global Python lists, not persistent data
  (`mock-erp/routers/finance.py:7-108`, `mock-erp/routers/hr.py:7-101`,
  `mock-erp/routers/inventory.py:8-132`). A posted purchase invoice merely
  adds a fixed name and draft docstatus; it neither stores the record nor posts
  a ledger (`mock-erp/routers/finance.py:97-102`).
- The service already follows useful ERPNext REST conventions: literal
  `/api/resource/{DocType}` endpoints, `{"data": ...}` response envelopes,
  Frappe-style error envelopes, and token/session/basic authentication
  (`mock-erp/openapi.yaml:1-18`, `mock-erp/dependencies.py:5-55`). Its
  hard-coded mock credentials and static sample records are unsuitable for a
  configurable persistent service.
- The OpenAPI document exposes Purchase Invoice, Payment Entry, Journal Entry,
  Employee, Department, Leave Application, Salary Slip, Item, Bin, and
  Purchase Order only (`mock-erp/openapi.yaml:20-438`). ERPBridge generates
  tools from this file (`Makefile:58-64`), so supported document routes and
  their response envelopes are a compatibility contract.
- The existing fixture plan owns the three integration-only routes needed by
  active SDK and upcoming external-plugin work. Those plans already depend on
  `/api/integration/echo` and `GET /api/resource/Plugin Fixture`
  (`.agents/plans/active/Plan-SDK-Integration-Testing.md:93-130`,
  `.agents/plans/upcoming/Plan-Generic-External-Plugins.md:164-166`). They
  must be delivered before, and remain independent from, the business-domain
  expansion.
- The Compose service has no durable Mock ERP volume or database configuration
  (`docker-compose.yml:1-39`). The Mock ERP project currently has only runtime
  FastAPI dependencies and no test group (`mock-erp/pyproject.toml:1-10`).
- The Mock ERP subtree has an isolated history beginning at `9169fcf` and eight
  path-scoped commits through `d81ebec`; it can be extracted with
  `git subtree split --prefix=mock-erp` before deletion from ERPBridge. The
  target `nmdra/mockerp` repository does not yet exist, while GitHub CLI
  authentication is available for its creation and GHCR workflow setup.
- ERPBridge currently builds Mock ERP from a local path in Compose and uses the
  local OpenAPI file in `Makefile:40-64`; documentation also links directly to
  `mock-erp/README.md` (`README.md:38-99`, `docs/docker.md:20-75`,
  `docs/faq.md:52-103`). All of these references must be migrated to the
  pinned image and versioned raw GitHub OpenAPI URL in the extraction task.
- ERPNext documents the standard goods sales cycle as Quotation → Sales Order
  → Delivery Note → Sales Invoice → Payment Entry; the delivery updates stock,
  while invoice and payment affect the GL
  ([Selling](https://docs.frappe.io/erpnext/selling)). Its ledger model derives
  balanced GL, payment, and—when applicable—stock entries from submitted
  business documents rather than relying on manual journal entries
  ([How Transactions Affect the Ledger](https://docs.frappe.io/erpnext/how-transactions-affect-the-ledger)).
- ERPNext Stock Entry covers material receipt, issue, transfer, manufacturing
  transfer/consumption, and manufacture
  ([Stock Entry](https://docs.frappe.io/erpnext/stock-entry)). Frappe HR
  supports the attendance states Present, Absent, On Leave, and Half Day
  ([Attendance](https://docs.frappe.io/hr/attendance)), approval-based leave
  ([Leave Application](https://docs.frappe.io/hr/leave-application)), and
  salary slips derived from salary structures and optionally attendance
  ([Salary Slip](https://docs.frappe.io/hr/salary-slip)).
- GitHub's Container Registry documentation supports publishing with the
  repository `GITHUB_TOKEN`, recommends explicit package permissions, and
  describes `org.opencontainers.image.source` linking
  ([Working with the Container registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)).

## Decisions

1. **One staged plan and one Mock ERP owner.** Repository extraction and image
   publication are prerequisites. Tasks 1–3 retain the narrow, deterministic
   integration-fixture contract as the prerequisite for the consuming plans;
   they execute in `../mockerp` after extraction. Tasks 4–16 then build the SCP
   legacy ERP in independently testable slices in that repository. No
   ERPBridge, SDK, or plugin task may edit the extracted Mock ERP source.
2. **SQLite is the authoritative business store.** Use Python's standard
   `sqlite3` module, explicit versioned migrations, foreign keys, transactions,
   and a repository/service boundary. Configure its path with
   `MOCK_ERP_DB_PATH`; production-like local runs use a mounted path, while
   every test uses a fresh temporary database. Do not use process-global lists
   for business records. The echo route's last payload remains intentionally
   process-local test-observation state, not ERP data. The image defaults to a
   safe container path and never stores the database in the image layer.
3. **Use a deterministic SCP seed, never real people or credentials.** Seed
   named company, branches, departments, designations, warehouses, chart of
   accounts, parties, products, BOMs, and representative documents with
   fictional data only. Seeded employees contain no NIC, passport, bank
   account, personal email, or document content; employee-document records
   contain only safe metadata. Credentials are injected from environment or
   Docker secrets, are redacted from logs, and are never committed or shown in
   documentation.
4. **Adopt ERPNext-compatible contracts, not its database.** Preserve
   `/api/resource/{DocType}`, `{"data": ...}`, canonical DocType names,
   `docstatus`, source-document references, and document names such as
   `EMP-00001`, `SO-2026-00001`, `SINV-2026-00001`, `MR-2026-00001`,
   `PO-2026-00001`, `PR-2026-00001`, `PINV-2026-00001`, `JE-2026-00001`,
   `PAY-2026-00001`, and `EXP-2026-00001`. Store a compact relational schema
   tailored to this mock; do not copy Frappe tables or call ERPNext in V1.
5. **Separate lifecycle from approval.** `docstatus` means ERPNext-style
   `0=DRAFT`, `1=SUBMITTED`, and `2=CANCELLED`. `workflow_state` independently
   records `DRAFT`, `PENDING_APPROVAL`, `APPROVED`, `REJECTED`, and operational
   completion states (for example `DELIVERED`, `PARTIALLY_PAID`). Only a
   permitted approved document may submit and create stock, payment, or GL
   effects. Cancellation creates compensating/reversal entries and audit
   history; it never silently deletes a posted transaction.
6. **Use document services for side effects.** All monetary values are handled
   as `Decimal` at service boundaries and stored as integer LKR minor units;
   never use binary floating-point for financial calculations. Sales invoices,
   purchase invoices, payments, payroll, assets, and inventory documents call
   one posting service that validates balanced debits and credits and writes
   immutable GL/payment/stock ledger rows in the same SQLite transaction.
   Manual Journal Entry is limited to authorized adjustments.
7. **Make access and approval data-driven.** Persist SCP users, roles, role
   assignments, approval limits, and approval actions. Implement the stated
   operational roles (System Administrator, HR Manager/Officer, Finance
   Manager/Accountant, Procurement Manager/Purchasing Officer, Sales
   Manager/Executive, Warehouse Manager/Storekeeper, Production
   Manager/Supervisor, Employee, and Director), with employee self-service
   restricted to the caller's own records. Purchase-order thresholds are SCP
   configuration: below LKR 100,000 procurement; LKR 100,000–500,000 finance;
   above LKR 500,000 director.
8. **Scope the manufacturing and local compliance honestly.** V1 supports
   single-level active BOMs, a Production Order, material transfer/consumption,
   finished-goods receipt, and scrap. It excludes MRP, capacity planning,
   routing/job cards, subcontracting, quality inspections, POS, CRM,
   recruitment, appraisals, projects, advanced tax, and automatic Sri Lankan
   PAYE/EPF/ETF/statutory filing. LKR, locations, product data, and configurable
   salary components make the scenario Sri Lankan without claiming legal
   payroll or tax compliance.
9. **Publish only supported operations.** Each delivered DocType gets explicit
   OpenAPI list/get/create/action operations, request validation, error cases,
   and generated-tool regression coverage. Do not pretend to implement the
   whole generic Frappe API, arbitrary filtering, arbitrary reports, or every
   ERPNext DocType. A future ERPNext adapter maps this documented subset; it is
   not part of this plan.
10. **Pin the downstream runtime contract.** ERPBridge uses
    `ghcr.io/nmdra/mockerp:0.2.1` and the matching
    `https://raw.githubusercontent.com/nmdra/mockerp/v0.2.1/openapi.yaml` rather
    than `latest` or a mutable branch. Upgrades change both values in one
    reviewed task, run the compatibility suite, and publish a new image tag.

## Scope

### In scope

- Existing authenticated integration fixtures and their OpenAPI/test contract
  in `nmdra/mockerp`.
- History-preserving repository extraction, standalone CI, versioned GHCR image
  publication, pinned ERPBridge Compose consumption, and versioned OpenAPI
  retrieval.
- SQLite configuration, migrations, deterministic reset/seed tooling, and
  durable Compose volume.
- SCP organization, users/roles, audit logs, configurable approvals, HR,
  payroll, parties, items, warehouses, inventory, finance, purchasing, sales,
  manufacturing-lite, assets, and focused reports.
- ERPNext-shaped names, resource paths, workflows, source references, and an
  explicit MockERP-to-ERPNext mapping document.
- Unit, service, HTTP, migration, OpenAPI, generated-tool, and end-to-end
  scenario tests; in-repository documentation, Unreleased changelog entries,
  and the matching public-docs repository plan/commit.

### Out of scope

- An ERPNext/Frappe installation, Frappe database/schema replication, or live
  ERPNext synchronization/migration.
- Real credentials, real employee/customer/supplier data, uploaded identity
  documents, or statutory payroll/tax compliance claims.
- Recruitment, performance, CRM, advanced manufacturing planning, projects,
  helpdesk, POS, complex tax automation, multi-company consolidation,
  background jobs, or production-scale concurrency/high availability.
- Any ERPBridge server, SDK, external-plugin, generated-schema, or binary
  implementation change beyond consuming this plan's documented API contract.
- Using a floating `latest` Mock ERP image, building Mock ERP from the ERPBridge
  repository, or copying Mock ERP source back into ERPBridge.

## Tasks

### Repository extraction and release prerequisites

The following prerequisite tasks must complete before the numbered application
 tasks. After Task C, paths such as `mock-erp/tests/...` in the historical
numbered descriptions mean the corresponding path under the standalone
`../mockerp` checkout; ERPBridge must not recreate those files.

- [x] **Task A: Extract Mock ERP history into `nmdra/mockerp`.** Create the new
  public GitHub repository `nmdra/mockerp` with `gh repo create`, run
  `git subtree split --prefix=mock-erp` from the ERPBridge history, and push the
  resulting root-level history to the new repository's `main` branch. Preserve
  the eight Mock ERP path commits and verify that the extracted tree contains
  `main.py`, `openapi.yaml`, `pyproject.toml`, `uv.lock`, `Dockerfile`, and
  `routers/`. Move the extended plan into the new repository as its active
  implementation plan, and add a standalone README section that identifies
  ERPBridge as a downstream consumer. Do not push credentials, `.venv`,
  `__pycache__`, database files, or generated schemas.

  **Seam:** Mock ERP path history → standalone Git repository root.

  **Files:** `../mockerp/` (new checkout),
  `../mockerp/.agents/plans/active/Plan-Mock-ERP.md` (new),
  `../mockerp/README.md`, `../mockerp/.gitignore`.

  **Verify:**

  ```bash
  test "$(git -C ../mockerp rev-list --count main)" -ge 8
  git -C ../mockerp ls-tree -r --name-only main | grep -E '^(main.py|openapi.yaml|pyproject.toml|uv.lock|Dockerfile|routers/)'
  ! git -C ../mockerp ls-tree -r --name-only main | grep -E '(^|/)(\.venv|__pycache__|.*\.db|schemas/)'
  git -C ../mockerp log --oneline --all | grep 'feat(mock-erp): rewrite mock ERP'
  ```

- [x] **Task B: Add standalone CI and publish the first pinned GHCR image.**
  Add Python test/lint jobs and a least-privilege GitHub Actions workflow using
  `docker/login-action` and `docker/build-push-action`. Publish
  `ghcr.io/nmdra/mockerp:0.1.0` and an immutable commit tag on release tag
  `v0.1.0`; grant only `contents: read` and `packages: write`, add the OCI
  source label to the Dockerfile, and make the package public. Keep runtime
  credentials environment-only. The workflow must run tests before pushing and
  must not publish `latest` as ERPBridge's contract.

  **Seam:** standalone repository tag → tested Docker image → GHCR package.

  **Files:** `../mockerp/.github/workflows/ci.yml` (new),
  `../mockerp/.github/workflows/publish-image.yml` (new),
  `../mockerp/Dockerfile`, `../mockerp/pyproject.toml`,
  `../mockerp/README.md`, `../mockerp/CHANGELOG.md` (new),
  `../mockerp/tests/test_smoke.py` (new),
  `../mockerp/.github/dependabot.yml` (new, if dependency updates are enabled).

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --group test pytest -q
  docker build -t mockerp:0.1.0 .
  docker run --rm -d --name mockerp-release-check -p 18081:8081 mockerp:0.1.0
  trap 'docker rm -f mockerp-release-check >/dev/null 2>&1 || true' EXIT
  curl --fail http://127.0.0.1:18081/health
  gh workflow run publish-image.yml --repo nmdra/mockerp --ref v0.1.0
  gh run watch --repo nmdra/mockerp --exit-status
  docker manifest inspect ghcr.io/nmdra/mockerp:0.1.0
  ```

- [x] **Task C: Make ERPBridge consume the pinned image and remote OpenAPI.**
  Replace the local `mock-erp` Compose build with
  `image: ${MOCK_ERP_IMAGE:-ghcr.io/nmdra/mockerp:0.1.0}`, add a named
  `mockerp-data` volume, set `MOCK_ERP_DB_PATH=/data/mockerp.db`, and keep the
  service name `mock-erp` so `ERP_BASE_URL=http://mock-erp:8081` remains stable.
  Replace local `mock-erp/openapi.yaml` generation with
  `MOCK_ERP_OPENAPI_URL` defaulting to the raw GitHub URL for `v0.1.0`; keep a
  deliberate version override for upgrades and fetch to a temporary ignored
  file before tool generation. Update README, Docker, onboarding, FAQ,
  environment, Makefile, and plan references so no ERPBridge command requires
  a local Mock ERP checkout. Keep the source repository and image version in
  one documented compatibility table.

  **Seam:** ERPBridge Compose service name/API URL → pinned GHCR image and
  pinned OpenAPI contract.

  **Files:** `docker-compose.yml`, `Makefile`, `README.md`, `docs/docker.md`,
  `docs/onboarding.md`, `docs/faq.md`, `docs/environment-variables.md`,
  `docs/README.md`, `.gitignore`, `.agents/plans/README.md`,
  `.agents/plans/upcoming/README.md`.

  **Verify:**

  ```bash
  docker compose config --quiet
  make build
  make generate-tools
  test -f /tmp/mockerp-openapi-v0.1.0.yaml
  rg -n 'ghcr.io/nmdra/mockerp:0.1.0|raw.githubusercontent.com/nmdra/mockerp/v0.1.0|MOCK_ERP_OPENAPI_URL' docker-compose.yml Makefile README.md docs
  if rg -n 'mock-erp/(README.md|openapi.yaml|Dockerfile|pyproject.toml)' README.md docs Makefile docker-compose.yml; then exit 1; fi
  ```

- [x] **Task 1: Establish the red integration-fixture and contract test suite.**
  Add a `test` dependency group with `pytest`, `httpx`, and `pyyaml`, update the
  lockfile, and add an in-process FastAPI test fixture that injects a temporary
  credential and database path. Before routes exist, assert the exact Plugin
  Fixture and echo/readback envelopes; token-auth rejection; non-object echo
  rejection; and absence of injected `role`, credential, timestamp, or request
  ID fields. Add a YAML contract test for the three paths and `TokenAuth`.

  **Seam:** FastAPI `app` boundary and checked-in OpenAPI contract.

  **Files:** `mock-erp/tests/conftest.py` (new),
  `mock-erp/tests/test_integration_fixtures.py` (new),
  `mock-erp/pyproject.toml`, `mock-erp/uv.lock`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  ```

  The first run must be red because the integration router does not exist.

- [x] **Task 2: Implement the authenticated deterministic integration fixtures.**
  Add a dedicated integration router and register it without changing the
  current finance, HR, or inventory endpoints. Implement the exact static
  `Plugin Fixture`, object-only echo, and process-local echo readback contract;
  authenticate every route through the existing dependency seam. Ensure
  invalid payloads are not recorded and errors retain the existing
  Frappe-style envelope.

  **Seam:** HTTP request → authentication dependency → integration router →
  JSON response.

  **Files:** `mock-erp/routers/integration.py` (new), `mock-erp/main.py`,
  `mock-erp/tests/test_integration_fixtures.py`, `mock-erp/README.md`,
  `CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  uv run python -m compileall -q main.py routers tests
  ```

- [x] **Task 3: Publish the fixture contract and transfer consumer ownership.**
  Add exact integration schemas, paths, success examples, `401`, and `422`
  responses to OpenAPI. Update the fixture guide without exposing a credential.
  Keep the SDK and plugin plans as consumers of the routes only; remove their
  Mock ERP file ownership and record this plan as the prerequisite. Publish
  patch release `v0.1.1` so the fixture contract is available in the image, then
  update ERPBridge's image and matching OpenAPI pin from `0.1.0` to `0.1.1`.
  Update both plan indexes to describe the broader staged plan while retaining
  this file's active path `../active/Plan-Mock-ERP.md` for those dependencies.

  **Seam:** checked-in OpenAPI contract → versioned MockERP image → ERPBridge
  tool generator and plan-to-plan fixture dependency.

  **Files:** `../mockerp/openapi.yaml`, `../mockerp/README.md`,
  `../mockerp/tests/test_integration_fixtures.py`,
  `../mockerp/CHANGELOG.md`, `../mockerp/pyproject.toml`, `../mockerp/uv.lock`,
  `.agents/plans/active/Plan-SDK-Integration-Testing.md`,
  `.agents/plans/upcoming/Plan-Generic-External-Plugins.md`,
  `.agents/plans/README.md`, `.agents/plans/upcoming/README.md`,
  `docker-compose.yml`, `Makefile`, `README.md`, `docs/docker.md`,
  `docs/environment-variables.md`, `docs/onboarding.md`, `docs/faq.md`,
  `docs/README.md`, `CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --group test pytest tests/test_integration_fixtures.py -q
  uv run python -c 'import yaml; yaml.safe_load(open("openapi.yaml", encoding="utf-8")); print("valid OpenAPI YAML")'
  cd ../ERPBridge
  docker compose config --quiet
  curl --fail --location --silent --show-error https://raw.githubusercontent.com/nmdra/mockerp/v0.1.1/openapi.yaml -o /tmp/mockerp-openapi-v0.1.1.yaml
  rg -n 'ghcr.io/nmdra/mockerp:0.1.1|raw.githubusercontent.com/nmdra/mockerp/v0.1.1|Plan-Mock-ERP|/api/integration/echo|Plugin Fixture' \
    docker-compose.yml Makefile README.md docs \
    .agents/plans/active/Plan-SDK-Integration-Testing.md \
    .agents/plans/upcoming/Plan-Generic-External-Plugins.md
  ```

- [x] **Task 4: Introduce the SQLite application platform and deterministic SCP bootstrap.**
  Write migration and lifecycle tests first. Add settings for an injected
  database path and credential source; enable foreign keys and transactional
  migrations; create a monotonic document-name sequence; make startup safe to
  repeat; and provide an explicit development-only reset/seed command. Mount a
  named Mock ERP data volume in Compose and ignore local database files. Seed
  only SCP's company profile, base LKR/fiscal settings, and safe service
  identities at this stage. Replace hard-coded credentials with environment or
  Docker-secret configuration and fail closed when absent.

  **Seam:** app lifespan → SQLite connection/migration manager → deterministic
  seed service; configuration → credential resolver.

  **Files:** `../mockerp/settings.py` (new), `../mockerp/database.py` (new),
  `../mockerp/migrations/001_platform.py` (new), `../mockerp/seed.py` (new),
  `../mockerp/dependencies.py`, `../mockerp/main.py`, `../mockerp/.gitignore`,
  `../mockerp/Dockerfile`, `docker-compose.yml`,
  `../mockerp/tests/test_database.py` (new),
  `../mockerp/tests/test_authentication.py` (new), `../mockerp/README.md`,
  `docs/docker.md`, `docs/environment-variables.md`, `CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_database.py tests/test_authentication.py -q
  uv run --locked --group test python -m compileall -q .
  cd ../ERPBridge
  docker compose config --quiet
  ```

- [x] **Task 5: Build SCP organization, authorization, approvals, and audit history.**
  Write service and HTTP tests before implementation. Add relational masters
  for company, branch, department hierarchy, designation, employment type,
  users, roles, user-role assignments, approval rules/requests/actions, and
  audit events. Seed all stated SCP locations and departments. Enforce role and
  employee-self-service checks; require configured sequential approvals and
  purchase thresholds; record immutable actor/action/before-after metadata
  without secrets or protected employee-document values.

  **Seam:** authenticated actor → authorization/workflow service → organization
  repository → audit event transaction.

  **Files:** `../mockerp/migrations/002_organization.py` (new),
  `../mockerp/repositories/organization.py` (new),
  `../mockerp/services/authorization.py` (new),
  `../mockerp/services/workflow.py` (new), `../mockerp/services/audit.py` (new),
  `../mockerp/routers/organization.py` (new), `../mockerp/seed.py`,
  `../mockerp/main.py`, `../mockerp/openapi.yaml`,
  `../mockerp/tests/test_organization.py` (new),
  `../mockerp/tests/test_workflow.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_organization.py tests/test_workflow.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 6: Implement the finance foundation and immutable double-entry ledger.**
  Start with red tests for chart hierarchy, Decimal/minor-unit conversion,
  balanced Journal Entry submission, prohibited unbalanced postings,
  cancellation reversals, AR/AP outstanding balances, and partial payment
  allocation. Add SCP chart-of-accounts masters and journal/payment documents;
  use one atomic posting service to write voucher-linked GL and payment ledger
  entries. Expose only authorized list/get/create/submit/cancel operations for
  Journal Entry and Payment Entry. Preserve the existing Purchase Invoice read
  envelope without altering its static implementation until Task 11 migrates
  that document and its item rows together.

  **Seam:** submitted finance document → accounting posting service → balanced
  GL/payment-ledger rows in one SQLite transaction.

  **Files:** `../mockerp/migrations/003_finance.py` (new),
  `../mockerp/repositories/finance.py` (new),
  `../mockerp/services/accounting.py` (new), `../mockerp/routers/finance.py`,
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_accounting.py` (new),
  `../mockerp/tests/test_finance_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_accounting.py tests/test_finance_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 7: Implement HR core and leave workflow.**
  Add employee, branch/department transfer, resignation, safe document
  metadata, attendance, leave type, leave allocation, and leave-application
  tables and services. Use the four Frappe HR attendance states, prevent future
  attendance and duplicate employee/date records, calculate leave days and
  balances, and route leave through employee → supervisor → HR approval.
  Migrate Employee, Department, and Leave Application reads from static data to
  SCP seed data and add create/approve/reject/cancel operations with audit
  records.

  **Seam:** employee/manager request → HR service → approval workflow and
  leave/attendance repository.

  **Files:** `../mockerp/migrations/004_hr.py` (new),
  `../mockerp/repositories/hr.py` (new), `../mockerp/services/hr.py` (new),
  `../mockerp/routers/hr.py`, `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_hr.py` (new), `../mockerp/tests/test_hr_routes.py` (new),
  `../mockerp/README.md`, `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_hr.py tests/test_hr_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 8: Add payroll, employee advances, and expense claims.**
  Add salary components, salary structures/assignments, salary slips/lines,
  employee advances, expense claims/lines, and reimbursement/payment
  references. Calculate a monthly slip from approved structure components and
  attendance/leave according to SCP configuration; post approved payroll and
  reimbursed claims through the finance posting service. Do not calculate or
  claim compliance for statutory PAYE, EPF, or ETF. Add the existing Salary
  Slip endpoint plus controlled payroll, advance, and expense-claim actions.

  **Seam:** approved payroll/expense document → HR calculation service →
  accounting posting service → GL and employee balance.

  **Files:** `../mockerp/migrations/005_payroll.py` (new),
  `../mockerp/repositories/payroll.py` (new), `../mockerp/services/payroll.py` (new),
  `../mockerp/services/expenses.py` (new), `../mockerp/routers/hr.py`,
  `../mockerp/routers/finance.py`, `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_payroll.py` (new),
  `../mockerp/tests/test_expenses.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_payroll.py tests/test_expenses.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 9: Add party, product, warehouse, and batch masters.**
  Add Customers/Suppliers with contacts and addresses; item groups; UOMs;
  Items; and the SCP warehouse tree: Katunayake Raw Material, WIP, Finished
  Goods, Scrap; Peliyagoda Main; Kandy DC; and Galle DC. Model purchase/sales/
  stock flags, valuation accounts, reorder values, batch-controlled items, and
  source/target warehouse eligibility. Migrate Item reads from static data;
  leave Bin readback unchanged until Task 10 derives it from the stock ledger.
  Seed raw materials, packaging, finished goods, imported products, office
  supplies, and spare parts, and keep only non-sensitive fictional party
  contacts.

  **Seam:** validated master command → master repository → supported DocType
  resource response.

  **Files:** `../mockerp/migrations/006_masters.py` (new),
  `../mockerp/repositories/masters.py` (new), `../mockerp/routers/inventory.py`,
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_masters.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_masters.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 10: Implement stock transactions, stock ledger, and Bin balances.**
  Create Stock Entry/Stock Entry Item and append-only Stock Ledger tables and
  services. Support Material Receipt, Material Issue, Material Transfer,
  Manufacturing Consumption, Manufacturing Receipt, and Stock Adjustment;
  validate item, warehouse, batch, quantity, and available stock; update Bin
  projections from ledger data rather than mutable static balances. Where SCP
  perpetual inventory applies, post stock value effects with the accounting
  service in the same transaction. Test no-negative-stock failures, transfer
  conservation, cancellation reversal, and every supported Stock Entry type.

  **Seam:** submitted Stock Entry → stock service → stock ledger/Bin projection
  and optional GL posting atomically.

  **Files:** `../mockerp/migrations/007_inventory.py` (new),
  `../mockerp/repositories/inventory.py` (new),
  `../mockerp/services/inventory.py` (new), `../mockerp/routers/inventory.py`,
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_inventory.py` (new),
  `../mockerp/tests/test_inventory_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_inventory.py tests/test_inventory_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 11: Implement procure-to-pay.**
  Add Material Request, Purchase Order, Purchase Receipt, and Purchase Invoice
  headers/items and their source links. Enforce the SCP flow Material Request →
  approved Purchase Order → Purchase Receipt → Purchase Invoice → Payment
  Entry; permit partial receipt, billing, and payment while keeping quantities,
  statuses, AP, stock, and source-document percentages consistent. Apply the
  configured PO approval thresholds and test duplicate/over-receipt/over-bill
  rejection plus ledger and stock side effects.

  **Seam:** submitted source document → procurement service → next-document
  mapper plus stock/accounting posting services.

  **Files:** `../mockerp/migrations/008_purchasing.py` (new),
  `../mockerp/repositories/purchasing.py` (new),
  `../mockerp/services/purchasing.py` (new), `../mockerp/routers/purchasing.py` (new),
  `../mockerp/routers/finance.py`, `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_purchasing.py` (new),
  `../mockerp/tests/test_purchasing_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_purchasing.py tests/test_purchasing_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 12: Implement order-to-cash.**
  Add optional Quotation, Sales Order, Delivery Note, Sales Invoice, and linked
  Payment Entry flows. Support partial delivery, partial billing, and partial
  payment; reserve/validate warehouse stock at the agreed point; make Delivery
  Note produce stock/COGS effects; and make Sales Invoice produce AR, income,
  and tax effects. Seed the Southern Hotels scenario and test source reference
  integrity, credit-limit decision, partial statuses, cancellation reversal,
  and fully balanced ledgers.

  **Seam:** submitted sales document → sales service → source progress,
  stock/COGS, AR/income, and payment-allocation services.

  **Files:** `../mockerp/migrations/009_sales.py` (new),
  `../mockerp/repositories/sales.py` (new), `../mockerp/services/sales.py` (new),
  `../mockerp/routers/sales.py` (new), `../mockerp/routers/finance.py`,
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_sales.py` (new),
  `../mockerp/tests/test_sales_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_sales.py tests/test_sales_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 13: Implement manufacturing-lite.**
  Add active single-level BOM/BOM Item and Production Order tables/services.
  Seed the Floor Cleaner 5L BOM. Allow a submitted production order to transfer
  raw materials to WIP, consume the actual or BOM quantities, receive finished
  goods, and optionally receive scrap. Require sufficient material and a valid
  active BOM; keep consumed/produced quantities and inventory valuation
  traceable to the production order. Do not add MRP, scheduling, routing, or
  job cards.

  **Seam:** production order action → manufacturing service → Stock Entry and
  accounting services with production-order source references.

  **Files:** `../mockerp/migrations/010_manufacturing.py` (new),
  `../mockerp/services/manufacturing.py` (new),
  `../mockerp/routers/manufacturing.py` (new), `../mockerp/openapi.yaml`,
  `../mockerp/seed.py`, `../mockerp/tests/test_manufacturing.py` (new),
  `../mockerp/tests/test_manufacturing_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_manufacturing.py tests/test_manufacturing_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 14: Add fixed assets and depreciation.**
  Add Asset Category, Asset, assignment/location, depreciation schedule,
  transfer, and disposal records. Seed a safe sample set of delivery trucks,
  forklifts, mixing/filling machines, computers, furniture, air conditioners,
  and generators. On authorized capitalization, scheduled depreciation, and
  disposal, create traceable balanced GL effects and audit events. Test an asset
  cannot be disposed twice and preserve the original history on transfer or
  disposal.

  **Seam:** approved asset event → asset service → depreciation schedule and
  accounting posting transaction.

  **Files:** `../mockerp/migrations/011_assets.py` (new),
  `../mockerp/services/assets.py` (new), `../mockerp/routers/assets.py` (new),
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_assets.py` (new),
  `../mockerp/tests/test_asset_routes.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_assets.py tests/test_asset_routes.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 15: Deliver constrained dashboards, audit readback, and the full deterministic scenario.**
  Add fixed, role-gated report endpoints for employee attendance/leave,
  AR/AP ageing and GL trial balance, stock by Item/Warehouse, sales order
  fulfilment, purchasing status, production consumption, and asset/depreciation
  summaries. Add paginated audit readback with redaction and date/type filters.
  Extend the seed to provide a coherent end-to-end SCP scenario spanning
  request-to-pay, order-to-cash, production, payroll, and asset events; test a
  reset produces byte-for-byte equivalent report results and all submitted
  vouchers balance.

  **Seam:** reporting query service → SQLite ledger/master projections →
  role-gated JSON report response.

  **Files:** `../mockerp/services/reports.py` (new),
  `../mockerp/routers/reports.py` (new), `../mockerp/routers/organization.py`,
  `../mockerp/openapi.yaml`, `../mockerp/seed.py`,
  `../mockerp/tests/test_reports.py` (new),
  `../mockerp/tests/test_end_to_end_scenario.py` (new), `../mockerp/README.md`,
  `../mockerp/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest tests/test_reports.py tests/test_end_to_end_scenario.py -q
  uv run --locked --group test python -m compileall -q .
  ```

- [x] **Task 16: Publish the supported contract and complete integration quality gates.**
  Document the SCP scenario, data-reset workflow, required environment-only
  credentials, security/redaction guarantees, supported document lifecycle,
  known non-goals, and exact ERPNext DocType/field/workflow mappings. Expand
  OpenAPI only for delivered operations, validate it, generate tools into a
  temporary ignored directory, and assert core generated operations still
  execute through ERPBridge against a seeded Mock ERP. Create and execute the
  required public-documentation plan and matching commit in
  `../erpbridge-docs`; update the public Mock ERP/Docker/API pages and that
  repository's changelog. Run focused Python checks, then the required
  repository test suite and only the lint scope covering changed Mock ERP/docs
  paths. Make one Conventional Commit per completed task; do not commit
  databases, generated schemas, binaries, or credentials.

  **Seam:** documented OpenAPI → generated ERPBridge tool → authenticated Mock
  ERP → deterministic SQLite-backed response.

  **Files:** `../mockerp/openapi.yaml`, `../mockerp/README.md`,
  `docs/mock-erp.md` (new), `docs/docker.md`, `docs/onboarding.md`,
  `docs/faq.md`, `docs/README.md`, `CHANGELOG.md`,
  `../mockerp/tests/test_openapi_contract.py` (new),
  `../erpbridge-docs/.agents/plans/Plan-mock-erp.md` (new),
  `../erpbridge-docs/docs/erpbridge/mock-erp.mdx` (new),
  `../erpbridge-docs/docs/erpbridge/api.mdx`,
  `../erpbridge-docs/docs/erpbridge/docker.mdx`,
  `../erpbridge-docs/docs/erpbridge/environment-variables.mdx`,
  `../erpbridge-docs/docs/erpbridge/onboarding.mdx`,
  `../erpbridge-docs/docs/erpbridge/faq.mdx`,
  `../erpbridge-docs/CHANGELOG.md`.

  **Verify:**

  ```bash
  cd ../mockerp
  uv run --locked --group test pytest -q
  uv run --locked --group test python -c 'import yaml; yaml.safe_load(open("openapi.yaml", encoding="utf-8")); print("valid OpenAPI YAML")'
  cd ../ERPBridge
  make test
  docker compose config --quiet
  git diff --check
  cd ../erpbridge-docs
  npm run build
  ```

## Verification

The plan is complete when:

- The three fixture endpoints retain their exact authenticated, non-PII
  contract and active SDK/plugin plans can use them without owning Mock ERP
  implementation files.
- A new Mock ERP instance migrates and seeds SCP reproducibly in SQLite; a
  restart preserves business data, while the explicit test/development reset
  produces the same safe fixture state.
- No production credential, real personal data, financial account number, or
  employee document content is committed, logged, returned through reports, or
  embedded in OpenAPI/examples.
- Every supported transactional document has canonical names, explicit
  lifecycle/approval rules, source references, authorization, audit history,
  tested validation failures, and Frappe-shaped API/error envelopes.
- Submitted sales, purchasing, inventory, payroll, and asset events create
  balanced, immutable accounting effects; stock movements create traceable
  item/warehouse ledger effects; cancellation reverses rather than deletes
  posted history.
- The seeded Sri Lankan manufacturing/distribution scenario includes SCP's
  locations, LKR accounts, HR leave/attendance, procure-to-pay, order-to-cash,
  Floor Cleaner production, and asset depreciation, all with deterministic
  report output.
- OpenAPI parses, describes only implemented operations, and still generates
  usable ERPBridge tools. Focused Mock ERP tests, the ERPBridge contract test,
  `make test`, relevant lint, documentation builds, and `git diff --check` are
  green.

## Open Questions

None for planning. Implementation must not promote this upcoming plan until a
review confirms the current ERPNext/Frappe version and public-docs navigation,
and explicitly approves the staged scope and SQLite durability model.
