#!/usr/bin/env python3
"""Recompute report metrics and vector plots from retained local evidence only."""
from pathlib import Path
from collections import Counter
import csv
import hashlib
import json
import math

ROOT = Path(__file__).resolve().parent
ARCHIVE = ROOT.parent
RESULTS = ARCHIVE / 'sources/research/evidence/evaluation/results'
DEMO = ARCHIVE / 'sources/erpbridge-demo/erpbridge-perf-evidence.json'
OUT = ROOT / 'src/generated'
OUT.mkdir(parents=True, exist_ok=True)

def quantile(values, probability):
    return sorted(values)[math.ceil(len(values) * probability) - 1]

def write(name, content):
    (OUT / name).write_text(content + '\n')

def coords(values):
    return ' '.join(f'({x},{y})' for x, y in values)

def plot(body, options):
    return '\\begin{tikzpicture}\n\\begin{axis}[' + options + ']\n' + body + '\n\\end{axis}\n\\end{tikzpicture}'

def csv_rows(name):
    with (RESULTS / name).open(newline='') as handle:
        return list(csv.DictReader(handle))

data = json.loads(DEMO.read_text())
samples = data['samples']
assert len(samples) == 100
assert all(row['ok'] is True and row['error'] is None for row in samples)
assert Counter(row['batch'] for row in samples) == {i: 10 for i in range(1, 11)}
counts = Counter(row['tool'] for row in samples)
assert len(counts) == 5 and set(counts.values()) == {20}
latencies = [row['ms'] for row in samples]
metrics = {
    'calls': len(samples), 'successes': sum(row['ok'] for row in samples),
    'elapsedSeconds': data['elapsedMs'] / 1000,
    'completionRatePerSecond': len(samples) / (data['elapsedMs'] / 1000),
    'medianMs': quantile(latencies, .5), 'p95Ms': quantile(latencies, .95),
    'p99Ms': quantile(latencies, .99), 'maxMs': max(latencies),
    'percentileMethod': 'nearest rank', 'perTool': {}, 'batches': []}
for name in counts:
    rows = [row for row in samples if row['tool'] == name]
    values = [row['ms'] for row in rows]
    metrics['perTool'][name] = {'calls': len(rows), 'rowsPerResponse': sorted({row['rows'] for row in rows}),
        'medianMs': quantile(values, .5), 'p95Ms': quantile(values, .95)}
for batch in range(1, 11):
    values = [row['ms'] for row in samples if row['batch'] == batch]
    metrics['batches'].append({'batch': batch, 'minMs': min(values),
        'medianMs': quantile(values, .5), 'maxMs': max(values)})
assert [metrics[k] for k in ['medianMs', 'p95Ms', 'p99Ms', 'maxMs']] == [361, 662, 691, 695]
assert round(metrics['completionRatePerSecond'], 2) == 8.37
(OUT / 'metrics.json').write_text(json.dumps(metrics, indent=2) + '\n')

# Empirical CDF, retaining duplicate observations in the ordered sample.
cdf = [(value, (i + 1) / len(latencies) * 100) for i, value in enumerate(sorted(latencies))]
write('cdf.tex', plot('\\addplot[ConversationBlue,very thick,const plot] coordinates {' + coords([(0, 0)] + cdf) + '};',
    r'xmin=0,xmax=750,ymin=0,ymax=100,xlabel={Client-observed latency (ms)},ylabel={Calls at or below latency (\%)},grid=major,ytick={0,25,50,75,100},height=72mm'))

names = list(counts)
median = [(i + 1, metrics['perTool'][name]['medianMs']) for i, name in enumerate(names)]
p95 = [(i + 1, metrics['perTool'][name]['p95Ms']) for i, name in enumerate(names)]
write('per-tool.tex', plot(
    '\\addplot[fill=ConversationBlue,draw=ConversationBlue] coordinates {' + coords(median) + '};\\addlegendentry{Median}\n' +
    '\\addplot[fill=white,draw=TaskAmber,line width=1pt] coordinates {' + coords(p95) + '};\\addlegendentry{p95}',
    r'ybar,bar width=9pt,ymin=0,ymax=750,ylabel={Client-observed latency (ms)},xtick={1,2,3,4,5},xticklabels={Items,Sales orders,Purchase orders,Assets,Search Item},x tick label style={font=\sffamily\scriptsize},ymajorgrids=true,legend style={at={(.5,1.04)},anchor=south,legend columns=2},height=70mm,enlarge x limits=.12'))

batch_lines = []
for row in metrics['batches']:
    batch_lines.append(r'\addplot[MutedInk,line width=1.5pt,forget plot] coordinates {' + coords([(row['batch'], row['minMs']), (row['batch'], row['maxMs'])]) + '};')
batch_lines.append(r'\addplot[ConversationBlue,only marks,mark=*,mark size=2.2pt] coordinates {' + coords([(r['batch'], r['medianMs']) for r in metrics['batches']]) + r'};\addlegendentry{Median; vertical line = min--max}')
write('batch.tex', plot('\n'.join(batch_lines),
    r'xmin=.5,xmax=10.5,ymin=0,ymax=750,xtick={1,...,10},xlabel={Recorded batch index (not elapsed time)},ylabel={Client-observed latency (ms)},ymajorgrids=true,legend pos=north east,height=65mm'))

transformation = csv_rows('transformation.csv')
assert len(transformation) == 8
checks = [('Syntax', 'syntactic_valid'), ('Name', 'tool_name_correct'), ('Input', 'input_schema_fidelity'),
    ('Location', 'parameter_locations_correct'), ('Method', 'method_correct'), ('Path', 'path_correct'),
    ('Credential', 'credential_ref_correct'), ('Response', 'response_mapping_correct'), ('All', 'all_structural_checks_pass')]
passes = [(i + 1, sum(row[field] == 'True' for row in transformation)) for i, (_, field) in enumerate(checks)]
write('transformation.tex', plot(
    r'\addplot[fill=ConversationBlue,draw=ConversationBlue] coordinates {' + coords(passes) + '};',
    r'ybar,bar width=14pt,ymin=0,ymax=9,ytick={0,2,4,6,8},ylabel={Passing rows / 8},xtick={1,...,9},xticklabels={Syntax,Name,Input,Location,Method,Path,Credential,Response,All},x tick label style={rotate=25,anchor=east,font=\sffamily\scriptsize},nodes near coords,ymajorgrids=true,height=65mm,enlarge x limits=.07'))

assert sum(row['all_structural_checks_pass'] == 'True' for row in transformation) == 7
invocations = sorted((RESULTS / 'task11/happy-path').glob('*/invocation.json'))
assert len(invocations) == 10
operations = [op for path in invocations for op in json.loads(path.read_text())['operations']]
assert len(operations) == 50 and all(op['status'] == 200 for op in operations)
governance = csv_rows('governance.csv')
assert len(governance) == 3 and all(row['observed'] == row['expected'] for row in governance)
evolution = csv_rows('evolution.csv')
assert len(evolution) == 24 and all(row['status'] == 'blocked' for row in evolution)
assert not csv_rows('agent-usability.csv')

manifest = json.loads((ARCHIVE / 'MANIFEST.json').read_text())
missing, mismatched = [], []
for entry in manifest['files']:
    path = ARCHIVE / entry['packagePath']
    if not path.is_file():
        missing.append(entry['packagePath'])
    elif hashlib.sha256(path.read_bytes()).hexdigest() != entry['sha256']:
        mismatched.append(entry['packagePath'])
# Inventory any files not represented in the original copy manifest.
listed = {entry['packagePath'] for entry in manifest['files']}
extra = [str(p.relative_to(ARCHIVE)) for p in (ARCHIVE / 'sources').rglob('*') if p.is_file() and str(p.relative_to(ARCHIVE)) not in listed]
validation = {'manifestCount': manifest['fileCount'], 'actualEntries': len(manifest['files']),
    'missingManifestPaths': missing, 'mismatchedHashes': mismatched, 'unlistedSourcePaths': extra,
    'inputSha256': {str(p.relative_to(ARCHIVE)): hashlib.sha256(p.read_bytes()).hexdigest()
        for p in [DEMO, RESULTS/'transformation.csv', RESULTS/'governance.csv', RESULTS/'overhead.csv']},
    'checks': {'demoSampleCounts': True, 'demoMetrics': True, 'historical50Outcomes': True,
        'transformation7of8': True, 'governance3of3': True, 'evolution24Blocked': True, 'usabilityNoRows': True}}
(OUT/'analysis-validation.json').write_text(json.dumps(validation, indent=2)+'\n')
print(json.dumps({'metrics':metrics,'archiveValidation':validation}, indent=2))
