# ToolPlane presentation evidence package

Start with [REPORT.md](REPORT.md). It contains findings, exact denominators, limitations, source links, slide guidance, and evaluation gaps.

## Status

**Preliminary historical evidence plus a separate demo read-performance record, not a release-verified evaluation.** Historical run-to-binary attribution is unknown. The demo files do not record the target deployment version. This export did not run new experiments or verify current ERPBridge behavior.

## Contents

- [REPORT.md](REPORT.md): detailed evidence report.
- [MANIFEST.json](MANIFEST.json): inventory and SHA-256 hashes for 116 copied source files.
- [sources/](sources/): original records copied without changes, with repository-relative paths preserved.
- [VALIDATION.md](VALIDATION.md): export-check results.

The copied sources include ten happy-path record directories, the retained combined boundary artifact, transformation results, governance and overhead CSVs, version-consistency rows, blocked/missing evaluation records, provenance documents, architecture documentation, and the existing capability comparison.

The added demo files are copied under `sources/erpbridge-demo/.agents/artifacts/`:

- [Read-performance report](sources/erpbridge-demo/.agents/artifacts/erpbridge-perf-report.md)
- [100-call raw evidence](sources/erpbridge-demo/.agents/artifacts/erpbridge-perf-evidence.json)

Their manifest entries include `originalRoot` to identify their external source location. Section 9A of the report explains this separate run and its limitations.

Original files remain in place. This curated package does not include a complete executable evaluation environment or downloaded external papers. Copied documents retain their original links, including links outside this package. Use the report for package-local navigation.

## Best presentation material

1. Integration feasibility: ten retained sequence records report 50 successful MCP operation outcomes across five ERPNext operations.
2. Structural conversion: seven of eight historical probe rows pass all structural checks; explain the response-mapping/oracle limitation.
3. Governance: three recorded allow/deny cases match expectations.
4. Separate demo read performance: 100/100 successful calls; 361 ms median, 662 ms p95, and 8.37 completed calls/s including pacing. This is not a sustained-capacity or direct-HTTP overhead benchmark.

Use overhead and comparison results only with the restrictions in the report. Do not claim maintenance savings, improved model task success, unchanged ERP code, or production reliability.

## Verify copied files

Run from this package directory:

```sh
python3 - <<'PY'
from pathlib import Path
import hashlib
import json

manifest = json.loads(Path('MANIFEST.json').read_text())
for entry in manifest['files']:
    path = Path(entry['packagePath'])
    assert path.stat().st_size == entry['bytes'], path
    assert hashlib.sha256(path.read_bytes()).hexdigest() == entry['sha256'], path
print(f"Verified {len(manifest['files'])} copied source files")
PY
```

Hashes verify copied bytes. They do not certify the original run provenance. The package manifest does not replace the incomplete historical release freeze.
