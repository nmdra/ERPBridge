# Agent-usability result schema

`agent-usability.csv` is a trial-level evidence file. The committed file contains
only its header because no model has been configured or run. The runner refuses to
create fabricated rows.

There must be exactly 20 rows for each `(task_id, condition, model_id)` pair.
Conditions are `raw_openapi`, `toolplane_generated`, `toolplane_reviewed`, and
`manual_mcp`. `task_success` is the primary metric and is populated only from the
adapter's observed evaluation result. Blank metrics mean that the adapter did not
return that measurement. `status=invalid_response` is not success.

The adapter must expose a fixed `ModelConfig` and return `ModelResponse`. Its
`evaluation.task_success` must be a boolean. The adapter is responsible for the
actual model/tool execution boundary and must provide observed outcomes, not
inferences. `provenance_ref` is supplied by the required provenance hook. Reset and
provenance hooks run before every trial. No provider SDK, credentials, or paid API
is included in this repository.
