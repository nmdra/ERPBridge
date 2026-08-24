# MockERP integration contract

ERPBridge uses [MockERP](https://github.com/nmdra/mockerp) as a versioned
ERPNext-compatible test service. The Compose service name is `mock-erp` and the
internal URL is `http://mock-erp:8081`.

## Pinned release

The default image and OpenAPI contract use the same immutable release:

- Image: `ghcr.io/nmdra/mockerp:0.2.1`
- Contract: `https://raw.githubusercontent.com/nmdra/mockerp/v0.2.1/openapi.yaml`

Upgrade both values together. Do not use `latest`.

## Credentials

MockERP fails closed when it has no credential source. Set one of these values
before starting Compose:

- `MOCK_ERP_CREDENTIALS_JSON` for local development.
- `MOCK_ERP_CREDENTIALS_FILE` for a mounted JSON Docker secret.

The credential document contains `credentials`, `sessions`, or `basic` lists.
Each identity has a role and an identity name. Do not commit credential values.

## Data and reset

MockERP stores data in `/data/mockerp.db`. Compose mounts this path in the
`mockerp-data` volume. Startup applies idempotent SQLite migrations and seeds
the fictional Serendib Consumer Products scenario.

The reset command is development-only:

```bash
MOCK_ERP_ENV=development MOCK_ERP_ALLOW_RESET=true \
  docker compose exec mock-erp python -m seed --reset
```

Do not run this command against a shared or production-like environment.

## Supported fixture groups

The API uses ERPNext-style `/api/resource/{DocType}` paths and `data` response
envelopes. The current release includes:

- organization, roles, approvals, and redacted audit events;
- LKR chart of accounts, Journal Entry, Payment Entry, AR/AP open items;
- employees, attendance, leave, payroll, advances, and expense claims;
- customers, suppliers, items, UOMs, warehouses, and append-only stock ledger;
- procure-to-pay and order-to-cash source-document flows;
- the Floor Cleaner 5L manufacturing flow and fixed assets; and
- role-gated trial balance, stock, ageing, operational, and audit reports.

`make generate-tools` downloads the pinned contract before tool generation. It
writes generated schemas to the ignored `schemas/` directory.

## Integration fixture

The authenticated `GET /api/resource/Plugin Fixture` endpoint returns a stable
source object for external-plugin tests. The authenticated
`POST /api/integration/echo` endpoint returns the submitted JSON object without
adding request metadata. Its readback endpoint is process-local test state.
