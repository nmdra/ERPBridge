# Evaluation evidence summary

This file is generated from existing CSV artifacts. It is not a claim of final evaluation completion.

## RQ1 (`results/transformation.csv`)
- Observed rows: **8**.
- `syntactic_valid`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `tool_name_correct`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `input_schema_fidelity`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `parameter_locations_correct`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `method_correct`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `path_correct`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `response_mapping_correct`: 7/8 (0.875), Wilson 95% CI [0.529, 0.978].
- `credential_ref_correct`: 8/8 (1.000), Wilson 95% CI [0.676, 1.000].
- `all_structural_checks_pass`: 7/8 (0.875), Wilson 95% CI [0.529, 0.978].
- `valid_call_status` categories: unavailable=8.

## RQ3-governance (`results/governance.csv`)
- Observed rows: **3**.
- `http_status`: n=3, median=200.000, IQR=0.000, p50=200.000, p95=200.000.
- `case_id` categories: reader-allowed=1, reader-missing-role=1, reader-wrong-role=1.
- `expected` categories: allow=1, deny=2.
- `is_error` categories: false=1, true=2.
- `observed` categories: allow=1, deny=2.

## RQ3-overhead (`results/overhead.csv`)
- Observed rows: **2**.
- `direct_p50_ms`: n=2, median=20.686, IQR=0.334, p50=20.352, p95=21.020.
- `direct_p95_ms`: n=2, median=23.560, IQR=0.445, p50=23.115, p95=24.005.
- `toolplane_p50_ms`: n=2, median=14.927, IQR=0.428, p50=14.498, p95=15.355.
- `toolplane_p95_ms`: n=2, median=24.201, IQR=0.537, p50=23.664, p95=24.738.
- `p95_overhead_ms`: n=2, median=0.641, IQR=0.982, p50=-0.341, p95=1.623.
- `runs` categories: 30=2.

## RQ4 (`results/evolution.csv`)
- Observed rows: **24**.
- `condition` categories: manual_mcp=6, raw_openapi=6, toolplane_generated=6, toolplane_reviewed=6.
- `status` categories: blocked=24.
- `notes` categories: no frozen service and repair/replay command supplied=24.

## RQ5 (`results/version-consistency.csv`)
- Observed rows: **2**.
- `advertised_has_limit_page_length`: 1/2 (0.500), Wilson 95% CI [0.095, 0.905].
- `agreement`: 1/2 (0.500), Wilson 95% CI [0.095, 0.905].
- `resolved_version` categories: 2.0.0=2.
- `v2_only_argument_http_status` categories: 200=1.

## Completeness and interpretation

- No trial-level run records were found under `runs/`; final-run completeness and the requested 10% replay cannot be verified from these artifacts.
- Empty or sparse CSVs are preserved as missing evidence, not converted into zero success.
- Existing rows are descriptive observations only. They do not establish the protocol's required 20-trial RQ2 matrix or a release freeze.
