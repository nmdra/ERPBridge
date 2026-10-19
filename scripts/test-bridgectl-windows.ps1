#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string] $BinaryPath
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) { throw 'This smoke check requires native Windows and PowerShell 7.' }
$sourceBinary = (Resolve-Path -LiteralPath $BinaryPath).Path
$ownedProcesses = [System.Collections.Generic.List[System.Diagnostics.Process]]::new()
$root = Join-Path ([System.IO.Path]::GetTempPath()) ('bridgectl smoke Ω ' + [guid]::NewGuid().ToString('N'))
$homeDir = Join-Path $root 'user profile Ω'
$tempDir = Join-Path $root 'temporary files'
$executable = Join-Path $root 'binary with spaces Ω/bridgectl.exe'
$http = $null

function Assert-That([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function Test-FilteredKey([string] $Key) {
    return $Key.StartsWith('BRIDGE_', [StringComparison]::OrdinalIgnoreCase) -or
        $Key.Equals('BRIDGECTL_MCP_CHILD_SNAPSHOT', [StringComparison]::OrdinalIgnoreCase) -or
        $Key.Equals('LOG_TO_STDERR', [StringComparison]::OrdinalIgnoreCase)
}

function New-RuntimeEnvironment($Source) {
    # Never change the parent environment. Keep Windows loader and host variables.
    $result = [System.Collections.Generic.Dictionary[string, string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($entry in $Source.GetEnumerator()) {
        if (-not (Test-FilteredKey ([string] $entry.Key))) {
            $result[[string] $entry.Key] = [string] $entry.Value
        }
    }
    $result['HOME'] = $homeDir
    $result['USERPROFILE'] = $homeDir
    $result['TEMP'] = $tempDir
    $result['TMP'] = $tempDir
    return ,$result
}

function Start-OwnedProcess([string[]] $CliArguments) {
    $info = [System.Diagnostics.ProcessStartInfo]::new()
    $info.FileName = $executable
    $info.WorkingDirectory = $root
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    $info.StandardOutputEncoding = [System.Text.Encoding]::UTF8
    $info.StandardErrorEncoding = [System.Text.Encoding]::UTF8
    foreach ($argument in $CliArguments) { $info.ArgumentList.Add($argument) }
    $info.Environment.Clear()
    $runtimeEnvironment = New-RuntimeEnvironment ([Environment]::GetEnvironmentVariables('Process'))
    foreach ($entry in $runtimeEnvironment.GetEnumerator()) { $info.Environment[$entry.Key] = $entry.Value }
    Assert-That (-not (@($info.Environment.Keys | Where-Object { Test-FilteredKey $_ }).Count)) 'Unsafe child environment.'
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $info
    try {
        Assert-That ($process.Start()) 'Could not start the CLI.'
    } catch {
        $process.Dispose()
        throw 'Could not start the CLI.'
    }
    $ownedProcesses.Add($process)
    return $process
}

function Invoke-CLI([string[]] $CliArguments, [int] $ExpectedExit = 0) {
    $process = Start-OwnedProcess $CliArguments
    $stdout = $process.StandardOutput.ReadToEndAsync()
    $stderr = $process.StandardError.ReadToEndAsync()
    Assert-That ($process.WaitForExit(30000)) 'CLI process timed out.'
    Assert-That ($stdout.Wait(5000) -and $stderr.Wait(5000)) 'CLI stream capture timed out.'
    # Do not echo raw streams, config, or Console capabilities on failure.
    Assert-That ($process.ExitCode -eq $ExpectedExit) 'CLI returned an unexpected exit code.'
    return [pscustomobject]@{ Stdout = $stdout.Result; Stderr = $stderr.Result }
}

function Get-LocalAsset([uri] $Uri, [string] $MediaPattern) {
    Assert-That ($Uri.Scheme -eq 'http' -and $Uri.Host -eq '127.0.0.1' -and $Uri.Port -eq $script:consolePort) 'Refused a non-fixture HTTP request.'
    $response = $null
    try {
        $response = $http.GetAsync($Uri).GetAwaiter().GetResult()
        Assert-That ($response.IsSuccessStatusCode) 'Console asset request failed.'
        Assert-That ([string] $response.Content.Headers.ContentType -match $MediaPattern) 'Unexpected Console asset content type.'
        $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        Assert-That (-not [string]::IsNullOrWhiteSpace($body)) 'Console asset was empty.'
        Assert-That ($body -notmatch 'fallback asset') 'Console served the fallback sentinel.'
        return $body
    } catch {
        # HttpClient exception text can include the request URL. Keep it private.
        throw 'Local Console asset validation failed.'
    } finally {
        if ($null -ne $response) { $response.Dispose() }
    }
}

try {
    foreach ($directory in @($root, $homeDir, $tempDir, (Split-Path $executable))) {
        [void] [System.IO.Directory]::CreateDirectory($directory)
    }
    Copy-Item -LiteralPath $sourceBinary -Destination $executable

    # Synthetic canaries test the same environment constructor, never a runtime.
    $synthetic = [System.Collections.Generic.Dictionary[string, string]]::new([StringComparer]::Ordinal)
    $synthetic['bRiDgE_API_TOKEN'] = 'synthetic-credential-canary'
    $synthetic['bridge_PASSWORD'] = 'synthetic-password-canary'
    $synthetic['bridgectl_mcp_child_snapshot'] = 'synthetic-snapshot-canary'
    $synthetic['log_to_stderr'] = 'synthetic-log-canary'
    foreach ($key in @('SystemRoot', 'PATH')) { $synthetic[$key] = [Environment]::GetEnvironmentVariable($key) }
    $filtered = New-RuntimeEnvironment $synthetic
    Assert-That (@($filtered.Keys | Where-Object { Test-FilteredKey $_ }).Count -eq 0) 'Environment filtering failed.'
    Assert-That (@($filtered.Values | Where-Object { $_ -like 'synthetic-*' }).Count -eq 0) 'A synthetic canary survived filtering.'
    foreach ($key in @('SystemRoot', 'PATH')) {
        Assert-That ($filtered[$key] -eq $synthetic[$key]) 'Required host environment was changed.'
    }
    Assert-That ($filtered['USERPROFILE'] -eq $homeDir -and $filtered['HOME'] -eq $homeDir) 'Home isolation failed.'

    $configDir = Join-Path $homeDir '.bridgectl'
    [void] [System.IO.Directory]::CreateDirectory($configDir)
    $configPath = Join-Path $configDir 'config.yaml'
    # All fixture endpoints are closed loopback port 1. No live deployment.
    [System.IO.File]::WriteAllText($configPath, @'
current-context: local
contexts:
  local:
    server: http://127.0.0.1:1
    mcp-server: http://127.0.0.1:1
    erp-base: http://127.0.0.1:1
  alternate:
    server: http://127.0.0.1:1
    mcp-server: http://127.0.0.1:1
    erp-base: http://127.0.0.1:1
'@, [System.Text.UTF8Encoding]::new($false))

    $version = Invoke-CLI @('--version')
    Assert-That ($version.Stdout -match 'bridgectl version' -and $version.Stderr -eq '') 'Version stream contract failed.'
    $completion = Invoke-CLI @('completion', 'powershell')
    Assert-That ($completion.Stdout -match 'Register-ArgumentCompleter' -and $completion.Stderr -eq '') 'PowerShell completion generation failed.'
    $null = Invoke-CLI @('context', 'set', 'alternate')
    $contexts = Invoke-CLI @('context', 'list', '--output', 'json')
    $items = @((ConvertFrom-Json $contexts.Stdout).items)
    Assert-That ($items.Count -eq 2) 'Context list did not use the isolated config.'
    Assert-That (@($items | Where-Object { $_.name -eq 'alternate' -and $_.current }).Count -eq 1) 'Context replacement was not persisted.'
    Assert-That ($contexts.Stderr -eq '') 'Context JSON wrote unexpected stderr.'
    $null = Invoke-CLI @('context', 'set', 'local')

    $skillDir = Join-Path $root 'skill destination Ω'
    $null = Invoke-CLI @('skill', 'install', '--dir', $skillDir)
    $skillFile = Join-Path $skillDir 'SKILL.md'
    Assert-That (Test-Path -LiteralPath $skillFile) 'Skill install did not create SKILL.md.'
    $originalSkill = [System.IO.File]::ReadAllText($skillFile)
    [System.IO.File]::WriteAllText($skillFile, 'replacement sentinel')
    $refused = Invoke-CLI @('skill', 'install', '--dir', $skillDir) 1
    Assert-That ($refused.Stdout -eq '' -and $refused.Stderr -match 'already exists') 'Skill refusal stream contract failed.'
    Assert-That ([System.IO.File]::ReadAllText($skillFile) -eq 'replacement sentinel') 'Refused install changed existing data.'
    $null = Invoke-CLI @('skill', 'install', '--dir', $skillDir, '--force')
    Assert-That ([System.IO.File]::ReadAllText($skillFile) -eq $originalSkill) 'Forced skill replacement failed.'
    Assert-That (@(Get-ChildItem -LiteralPath $root -Force | Where-Object { $_.Name -match '^\.skill destination.*\.(tmp|backup)-' }).Count -eq 0) 'Skill staging files were not cleaned up.'
    $invalid = Invoke-CLI @('not-a-bridgectl-command') 1
    Assert-That ($invalid.Stdout -eq '' -and $invalid.Stderr -match 'unknown command') 'Invalid command stream contract failed.'

    $console = Start-OwnedProcess @('web', '--no-open', '--listen', '127.0.0.1:0')
    $consoleErrors = $console.StandardError.ReadToEndAsync()
    $lineTask = $console.StandardOutput.ReadLineAsync()
    Assert-That ($lineTask.Wait(15000)) 'Console readiness timed out.'
    $line = $lineTask.Result
    Assert-That ($null -ne $line -and $line.StartsWith('ERPBridge Console: ')) 'Console did not report readiness.'
    $capability = [uri] $line.Substring('ERPBridge Console: '.Length)
    Assert-That ($capability.Scheme -eq 'http' -and $capability.Host -eq '127.0.0.1' -and $capability.Port -gt 0 -and $capability.Fragment.Length -gt 1) 'Console capability format was unexpected.'
    $script:consolePort = $capability.Port
    $baseUri = [uri] ('http://127.0.0.1:' + $consolePort + '/')
    $handler = [System.Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = $false
    $handler.UseProxy = $false
    $http = [System.Net.Http.HttpClient]::new($handler)
    $http.Timeout = [TimeSpan]::FromSeconds(5)
    $html = Get-LocalAsset $baseUri '^text/html'
    Assert-That ($html -match 'ERPBridge Console') 'Embedded HTML title was missing.'
    $assetMatches = [regex]::Matches($html, '(?:src|href)=["''](/assets/[^"''?#]+\.(?:js|css))["'']')
    $jsCount = 0
    $cssCount = 0
    foreach ($match in $assetMatches) {
        $path = $match.Groups[1].Value
        $assetUri = [uri]::new($baseUri, $path)
        if ($path.EndsWith('.js')) {
            $body = Get-LocalAsset $assetUri '(javascript|ecmascript)'
            $jsCount++
        } else {
            $body = Get-LocalAsset $assetUri '^text/css'
            $cssCount++
        }
        Assert-That ($body -notmatch '<!doctype|<html') 'Asset request returned the HTML fallback.'
    }
    Assert-That ($jsCount -gt 0 -and $cssCount -gt 0) 'Production JS/CSS references were missing.'
    Assert-That (-not $console.HasExited) 'Console exited during asset checks.'
    Write-Host ('Windows CLI and embedded Console smoke checks passed. PowerShell ' + $PSVersionTable.PSVersion + '; ' + [Environment]::OSVersion.VersionString)
} finally {
    if ($null -ne $http) { $http.Dispose() }
    $cleanupFailed = $false
    foreach ($process in $ownedProcesses) {
        try {
            if (-not $process.HasExited) { $process.Kill($true) }
            if (-not $process.WaitForExit(10000)) { $cleanupFailed = $true }
        } catch {
            $cleanupFailed = $true
        } finally { $process.Dispose() }
    }
    # No parent environment entries were changed, so presence and values stay intact.
    try {
        if (Test-Path -LiteralPath $root) { Remove-Item -LiteralPath $root -Recurse -Force }
    } catch { $cleanupFailed = $true }
    if ($cleanupFailed) { throw 'Could not clean up all owned smoke-test resources.' }
}
