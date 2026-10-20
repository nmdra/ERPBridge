# ERPBridge Evaluation Report

Deliverable: `../ERPBridge-Evaluation-Report.pdf` (14 pages).

## Rebuild

Run from this directory:

```sh
python3 analyze_evidence.py
tectonic -X build --keep-logs -p
cp build/erpbridge-evaluation/erpbridge-evaluation.pdf ../ERPBridge-Evaluation-Report.pdf
```

The analysis expects the supplied evidence in `../sources/`. It uses Python's standard library and does not access a live ERP or model provider. Plot coordinates and summaries are generated in `src/generated/`.

## Sources and QA

- `src/content/`: report sections.
- `src/design/evaluation.tex`: shared report styling.
- `src/figures/`: vector evidence and workload diagrams.
- `analyze_evidence.py`: numerical checks and vector-chart generation.
- `src/generated/metrics.json`: recomputed performance statistics.
- `src/generated/analysis-validation.json`: archive discrepancies and input hashes.
- `QA-NOTES.md`: acceptance record and remaining limits.
- `qa/final/`: structural report, rendered pages, and contact sheet.
- `build/erpbridge-evaluation/`: final PDF and retained engine log.

This is a standalone designed evidence report, not the IEEE manuscript. Original records and their existing manifest remain unchanged. The PDF documents stale manifest paths and the absent copied manuscript section. Historical release attribution remains unknown.
