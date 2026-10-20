# RQ3 governance and overhead collection status

The deterministic runners are ready, but this workspace check could not collect
live RQ3 observations. The local ToolPlane and ERPNext services were not
available with the required credential environment variables
`GOVERNANCE_READER_TOKEN`, `GOVERNANCE_INVENTORY_TOKEN`, `ERPNEXT_API_TOKEN`,
and `API_AUTH_TOKEN`. The runners exit with a blocker status and do not encode
missing calls as denies, failures, or zero latency.

`policy-matrix.json` contains three oracle cases, including an allowed role, a
wrong-role denial, and a missing-role denial. Run `governance.py --dry-run` to
validate the matrix without credentials. Run `overhead.py --dry-run` to check
the required 30-run configuration. A final overhead result also requires an
explicit cache/auth configuration and a running loopback ERPNext and
ToolPlane deployment. Existing CSV files are not treated as newly collected
by this check because their raw run provenance was not available here.
