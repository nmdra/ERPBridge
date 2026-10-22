# bridgectl on Windows

Use the Windows CLI to manage an already configured remote ToolPlane deployment.
Ordinary CLI use does not require administrator access, Bash, Make, Docker, or a
local server. Native Windows server hosting, Windows services, and porting the
Makefile or Compose bootstrap are outside this guide. Linux behavior and
installation stay unchanged.

## Validation status

Windows ZIP artifacts already exist for x64 and ARM64. Native x64 tests and
PowerShell smoke checks passed in [GitHub Actions](https://github.com/nmdra/ERPBridge/actions/runs/39213646691)
on Windows NT 10.0.26100.0 with PowerShell 7.6.6. These results apply to the implementation
branch, not older release binaries. ARM64 has cross-build coverage only.
Its runtime behavior is not verified. Cross-compilation does not prove runtime
behavior.
Default-browser launch and interactive Ctrl+C shutdown remain manual Windows
gates. The installation examples target Windows PowerShell 5.1 and PowerShell
7; automated shell checks use PowerShell 7. Native execution of these documented
steps is still pending. When you complete those checks, record the Windows version, PowerShell version,
and CPU architecture.

## Install a release ZIP

Choose `bridgectl_Windows_x86_64.zip` for x64 or
`bridgectl_Windows_arm64.zip` for ARM64. Replace the example tag with the exact
published release you intend to install. Archive names do not contain the tag;
the release URL does. Do not assume an older release contains unreleased fixes.

Run these commands in PowerShell. They download files, not a script to execute:

```powershell
$Tag = 'v0.6.0-alpha.2' # Replace with the intended published release tag.
$Archive = 'bridgectl_Windows_x86_64.zip' # Or bridgectl_Windows_arm64.zip.
$Release = "https://github.com/nmdra/ERPBridge/releases/download/$Tag"
$Download = Join-Path $env:TEMP ([guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $Download -ErrorAction Stop | Out-Null
$Zip = Join-Path $Download $Archive
$Checksums = Join-Path $Download 'checksums.txt'
Invoke-WebRequest -UseBasicParsing -Uri "$Release/$Archive" -OutFile $Zip -ErrorAction Stop
Invoke-WebRequest -UseBasicParsing -Uri "$Release/checksums.txt" -OutFile $Checksums -ErrorAction Stop

# Select exactly the entry for this archive, not another platform's checksum.
$Entries = @(Get-Content -LiteralPath $Checksums | Where-Object {
    $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($Archive) + '$')
})
if ($Entries.Count -ne 1) { throw 'Expected exactly one matching checksum entry.' }
$Expected = ($Entries[0] -split '\s+')[0]
$Actual = (Get-FileHash -LiteralPath $Zip -Algorithm SHA256).Hash
if ($Actual -ine $Expected) { throw 'SHA256 mismatch. Do not extract or run this archive.' }

$InstallDir = Join-Path $env:LOCALAPPDATA "ERPBridge CLI\$Tag"
if (Test-Path -LiteralPath $InstallDir) { throw 'Install directory already exists. Choose a new directory.' }
Expand-Archive -LiteralPath $Zip -DestinationPath $InstallDir -ErrorAction Stop
$Bridge = Join-Path $InstallDir 'bridgectl.exe'
& "$Bridge" version
if ($LASTEXITCODE -ne 0) { throw 'bridgectl version failed.' }
```

A checksum fetched from the same release detects corruption, not independent
publisher authenticity. Review the release source. Apply your organization's
software policy. Keep the ZIP and matching checksum if required for auditing.
The quoted path and PowerShell call operator `&` handle spaces in executable
paths. Do not wrap the command in `cmd /c` or an extra shell.

### Optional user PATH and completion

Direct invocation with `& "$Bridge"` needs no PATH change. To add this version
for future shells without replacing existing user PATH entries:

```powershell
$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (@($UserPath -split ';') -notcontains $InstallDir) {
    $NewPath = if ([string]::IsNullOrEmpty($UserPath)) { $InstallDir } else { $UserPath.TrimEnd(';') + ';' + $InstallDir }
    [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
}
```

Open a new terminal to receive the updated user PATH. Existing entries remain.
An older `bridgectl` earlier in PATH can still take precedence. Use the absolute path to
select a specific version.

Generate and load completion in the current session only if local script policy
permits it:

```powershell
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
$Completion = Join-Path $InstallDir 'bridgectl-completion.ps1'
$CompletionLines = & "$Bridge" completion powershell
if ($LASTEXITCODE -ne 0) { throw 'Completion generation failed.' }
[IO.File]::WriteAllLines($Completion, [string[]]$CompletionLines, $Utf8NoBom)
. "$Completion"
```

After you review this exact file, add a dot-source line for it to your own
PowerShell `$PROFILE` for future sessions. Do not change machine execution
policy. If organizational policy blocks scripts, omit completion and use the
CLI directly.

## Configure a context without storing credentials

Windows uses `%USERPROFILE%\.bridgectl\config.yaml` and per-context API
registries at `%USERPROFILE%\.bridgectl\registries\<context>.json`.
Use a unique context name, such as `staging`.

Have your approved secret manager or parent process inject
`$env:BRIDGE_API_TOKEN` into the shell environment before starting the CLI.
Do not put the token value in commands, history, client files, or screenshots.
The CLI bearer token authenticates to ToolPlane, not to the downstream ERP.
Downstream credential references resolve on the remote server by default.

The single-quoted here-string below keeps `${BRIDGE_API_TOKEN}` literal in YAML.
The CLI expands it at runtime. Replace the example endpoint with your approved
remote deployment. This example refuses to overwrite an existing configuration:

```powershell
if ([string]::IsNullOrEmpty($env:BRIDGE_API_TOKEN)) { throw 'Inject BRIDGE_API_TOKEN through the approved secret provider first.' }
$ConfigDir = Join-Path $env:USERPROFILE '.bridgectl'
$ConfigFile = Join-Path $ConfigDir 'config.yaml'
if (Test-Path -LiteralPath $ConfigFile) { throw 'Config exists. Back it up and edit it instead.' }
New-Item -ItemType Directory -Path $ConfigDir -Force -ErrorAction Stop | Out-Null
$Yaml = @'
current-context: staging
contexts:
  staging:
    server: https://bridge.example.com
    mcp-server: https://bridge.example.com
    api-token: ${BRIDGE_API_TOKEN}
'@
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($ConfigFile, $Yaml, $Utf8NoBom)
& "$Bridge" context list
if ($LASTEXITCODE -ne 0) { throw 'Context configuration failed.' }
```

Use explicit UTF-8 without a BOM for YAML and manifest files on both PowerShell
5.1 and 7. Do not use PowerShell 5.1's default `>` or `Out-File` encoding for
these files. Existing `BRIDGE_*` environment overrides can change the effective
context and endpoints. Make sure that you know which overrides are present. Do not print secrets.
See [environment precedence](./environment-variables.md#cli-variables-bridgectl).

### Windows context safety and recovery

On Windows, a config containing both `prod` and `Prod` is refused before
registry access or management MCP child execution, even if another context is
selected. Context selection writes also refuse an ambiguous saved document.
Choose names that differ by more than letter case. Registry paths reject
reserved device stems, including `CON`, `con`, `NUL.test`, `COM1`, and `LPT9`.
`console` and `com10` are not device names. Unsafe device names cannot be used
to construct a registry path. These are Windows-only checks; Linux naming and
case-sensitive registry behavior stay unchanged.

If the CLI refuses the configuration, stop CLI, Console, and MCP processes. Back up `config.yaml` and the
registry directory privately. Review the saved context keys and any existing
registry files, then manually choose distinct safe names and update the matching
config references and registry filenames. Do not merge or delete registry data.
A case-insensitive filesystem may already have aliased two old names; do not
assume separate recoverable files exist. The CLI does not rename or migrate
state automatically. Before you restart processes, make sure that the mapping is correct.

Windows directory ACLs control who can read these files. Unix mode `0600` does
not guarantee an owner-only Windows ACL. Use a private user directory. If necessary, ask your administrator to inspect
its inherited ACLs. Do not assume config
replacement is crash-atomic on Windows. If replacement fails, it must report an error. It must preserve the original.
It must remove temporary files where possible.

## Local validation, skills, and Console

Use reviewed manifest files rather than inline JSON native-command arguments.
For example, with an existing reviewed file under a path containing spaces:

```powershell
$Manifest = Join-Path $env:USERPROFILE 'ERP manifests\reviewed-tool.yaml'
& "$Bridge" tool validate --context staging -f "$Manifest"
if ($LASTEXITCODE -ne 0) { throw 'Manifest validation failed.' }
```

Validation does not apply the manifest. Remote reads, probes, and mutations need
their own authorization. Do not run `api test`, `tool apply`, or a live probe
merely to make sure that installation succeeded.

```powershell
& "$Bridge" skill install
# Or, from the intended project directory:
& "$Bridge" skill install --project
# Or select an exact destination:
& "$Bridge" skill install --dir "$env:USERPROFILE\Agent skills\bridgectl-ops"
```

Global skills go to `%USERPROFILE%\.agents\skills\bridgectl-ops`; project skills
go to `.agents\skills\bridgectl-ops` below the current directory. The CLI does not silently overwrite existing
content. Before you use `--force`, review that content.

```powershell
& "$Bridge" web --no-open
```

Open the printed local Console URL privately. Its fragment contains a capability.
Do not paste it into logs, tickets, or chat. By default, `web` attempts to open the
default browser. `--no-open` avoids that step. Keep the process running while
using the Console. Close it with Ctrl+C. Browser launch and interactive Ctrl+C
cleanup, including absence of leftover child processes, need manual Windows
verification. See [Console security](./web-console.md#security-boundary).

## Management MCP on Windows

Use a direct absolute executable path and an argument array, not a shell string.
For Codex TOML, escape backslashes in basic strings:

```toml
[mcp_servers.erpbridge_manage]
command = "C:\\Users\\Alice\\AppData\\Local\\ERPBridge CLI\\v0.6.0-alpha.2\\bridgectl.exe"
args = ["mcp", "--stdio", "--context", "staging"]
env_vars = ["BRIDGE_API_TOKEN"]
```

A JSON client with the common `mcpServers` structure uses the same escaping:

```json
{
  "mcpServers": {
    "erpbridge_manage": {
      "command": "C:\\Users\\Alice\\AppData\\Local\\ERPBridge CLI\\v0.6.0-alpha.2\\bridgectl.exe",
      "args": ["mcp", "--stdio", "--context", "staging"]
    }
  }
}
```

Replace the user and release directory with your actual path. Client schemas
vary. Configure host-environment inheritance according to your client's
instructions. Do not add a token value to an `env` map. The parent application
must already have the credential. Some clients require an explicit
host-variable forwarding allow-list. Keep stdout exclusively for MCP protocol
traffic. Defaults are read-only. See [management MCP](./bridgectl-mcp.md) for
write grants, redaction, and `outcome_unknown` recovery.

After you change a saved context, endpoint, or credential, restart the MCP
process. A session retains its launch-time selection. Closing stdin (EOF) ends the
stdio session. You must make sure that interactive Ctrl+C works on native Windows.
Unix SIGTERM behavior is not a Windows guarantee.

## Troubleshooting

- **Executable not found:** make sure that the extracted `.exe` path is correct.
  Use `&` with a quoted absolute path. Reopen the terminal after a user PATH change.
- **Checksum missing or mismatched:** make sure that the tag, architecture, and
  exact archive entry are correct. Stop. Do not bypass verification.
- **Wrong target or authentication failure:** inspect configured context and
  environment override presence privately. Reinject the credential through the
  approved provider and restart long-running Console/MCP processes.
- **Config or manifest parse failure:** write UTF-8 without a BOM. Keep YAML
  placeholders in a single-quoted here-string.
- **Context refused:** follow manual backup and distinct-name recovery above.
  Do not delete registries to silence the error.
- **Browser does not open:** use `web --no-open` and open its URL privately.
- **Access denied or replacement failure:** make sure that inherited directory
  ACLs permit access. Make sure that no other process blocks access to the file. Keep the original file; do not delete it as
  a replacement workaround.
