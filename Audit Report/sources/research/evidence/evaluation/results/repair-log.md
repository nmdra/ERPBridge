# Contract-evolution repair log

## Deterministic runner readiness

`evaluation/runner/evolution.py` validates all six mutation definitions and expands
four core conditions per mutation. It records repair duration, changed lines,
manual operations, broken tools, and restored task success only when an explicit
repair/replay command is supplied through `--repair-command`. The command must print a final JSON object containing a
boolean `restored_success` field and may include `changed_lines`,
`manual_operations`, and `broken_tools`; an exit code alone is never treated as
restored behavior.

## Blocker for observed RQ4 measurements

No frozen `toolplane-eval-v1.0` release, local ERP service, or reproducible
per-condition repair/replay procedure was available in this workspace at the
time of this check. Therefore no repair time, edit count, broken-tool count, or
restored-success result is reported here. The runner's `--record-blockers` mode
can create one explicitly blocked row for each mutation-condition pair. A
successful exit from an arbitrary command is not treated as evidence unless the
command itself applies the mutation and replays the corresponding task oracle.

The six applicable mutation files are: renamed parameter, newly required field,
server URL change, authentication-header change, response-envelope change, and
deprecated operation.
