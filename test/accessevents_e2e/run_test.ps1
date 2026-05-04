# =============================================================================
# run_test.ps1 — MMA2 Access Events End-to-End Test (Windows / PowerShell)
#
# What it does
# ------------
# 1. Builds the mma2 binary (go build).
# 2. Starts mma2 with test_config.yaml in a background job.
# 3. Connects to /events and saves the NDJSON stream to evidence\events.ndjson.
# 4. Runs traffic_gen.py to produce allowed + denied access events.
# 5. Waits for the aggregation window to expire.
# 6. Prints the captured NDJSON and validates key fields.
# 7. Cleans up.
#
# Requirements
# ------------
#   Go   1.21+  (go.exe on PATH)
#   Python 3.x  (python.exe on PATH)
#   curl        (bundled with Windows 10+)
#
# Usage
# -----
#   cd test\accessevents_e2e
#   .\run_test.ps1
#
# Evidence is written to:
#   test\accessevents_e2e\evidence\events.ndjson
#   test\accessevents_e2e\evidence\mma2.log
# =============================================================================

$ErrorActionPreference = "Stop"

$ScriptDir   = $PSScriptRoot
$RepoRoot    = Resolve-Path "$ScriptDir\..\.."
$EvidenceDir = "$ScriptDir\evidence"
$Config      = "$ScriptDir\test_config.yaml"
$Binary      = "$ScriptDir\mma2.exe"
$Mma2Log     = "$EvidenceDir\mma2.log"
$EventsFile  = "$EvidenceDir\events.ndjson"
$ModbusPort  = if ($env:MODBUS_PORT) { [int]$env:MODBUS_PORT } else { 5020 }  # override to 502 in Docker
$HttpPort    = if ($env:HTTP_PORT)   { [int]$env:HTTP_PORT   } else { 9090 }

$Mma2Process   = $null
$EventsJob     = $null

function Banner($msg) { Write-Host ""; Write-Host ">>> $msg"; Write-Host "" }

function Wait-Port($port, $retries = 20, $delayMs = 500) {
    Write-Host -NoNewline "    Waiting for :$port"
    for ($i = 0; $i -lt $retries; $i++) {
        try {
            $tcp = New-Object System.Net.Sockets.TcpClient
            $tcp.Connect("127.0.0.1", $port)
            $tcp.Close()
            Write-Host " — ready"
            return
        } catch {
            Write-Host -NoNewline "."
            Start-Sleep -Milliseconds $delayMs
        }
    }
    Write-Host " — TIMED OUT"
    throw "Port $port did not open in time"
}

try {
    # -----------------------------------------------------------------------
    # Step 1 — Build
    # -----------------------------------------------------------------------
    Banner "Step 1 — Build mma2"
    Push-Location $RepoRoot
    go build -o $Binary ./cmd/mma2
    Pop-Location
    Write-Host "    Binary: $Binary"

    # -----------------------------------------------------------------------
    # Step 2 — Prepare evidence directory
    # -----------------------------------------------------------------------
    New-Item -ItemType Directory -Force -Path $EvidenceDir | Out-Null
    "" | Set-Content $EventsFile -Encoding UTF8
    "" | Set-Content $Mma2Log   -Encoding UTF8

    # -----------------------------------------------------------------------
    # Step 3 — Start mma2
    # -----------------------------------------------------------------------
    Banner "Step 2 — Start mma2"
    $Mma2Process = Start-Process -FilePath $Binary `
        -ArgumentList $Config `
        -RedirectStandardOutput $Mma2Log `
        -RedirectStandardError  "$EvidenceDir\mma2_err.log" `
        -PassThru -NoNewWindow

    Write-Host "    mma2 PID: $($Mma2Process.Id)"

    Wait-Port $ModbusPort
    Wait-Port $HttpPort

    # -----------------------------------------------------------------------
    # Step 4 — Start /events capture in background job
    # -----------------------------------------------------------------------
    Banner "Step 3 — Connect to /events (background capture)"
    $EventsJob = Start-Job -ScriptBlock {
        param($url, $file)
        # Stream NDJSON line by line and append to file.
        $req = [System.Net.HttpWebRequest]::Create($url)
        $req.Timeout = [System.Threading.Timeout]::Infinite
        $resp   = $req.GetResponse()
        $stream = $resp.GetResponseStream()
        $reader = New-Object System.IO.StreamReader($stream)
        while (-not $reader.EndOfStream) {
            $line = $reader.ReadLine()
            if ($line) { Add-Content -Path $file -Value $line -Encoding UTF8 }
        }
    } -ArgumentList "http://127.0.0.1:$HttpPort/events", $EventsFile

    Write-Host "    Events capture job ID: $($EventsJob.Id)   output: $EventsFile"
    Start-Sleep -Milliseconds 500   # let the subscription register

    # -----------------------------------------------------------------------
    # Step 5 — Generate traffic
    # -----------------------------------------------------------------------
    Banner "Step 4 — Generate traffic (traffic_gen.py)"
    $TrafficLog = "$EvidenceDir\traffic_gen.log"
    python "$ScriptDir\traffic_gen.py" 127.0.0.1 $ModbusPort 2>&1 | Tee-Object -FilePath $TrafficLog

    # -----------------------------------------------------------------------
    # Step 6 — Wait for window expiry
    # -----------------------------------------------------------------------
    Banner "Step 5 — Wait 7 s for 5 s aggregation window to expire"
    Start-Sleep -Seconds 7
    Write-Host "    Done waiting."

    # -----------------------------------------------------------------------
    # Step 7 — Show captured events
    # -----------------------------------------------------------------------
    Banner "Step 6 — Captured /events output (evidence\events.ndjson)"
    Write-Host "--------------------------------------------------------------------"
    Get-Content $EventsFile | Where-Object { $_.Trim() } | ForEach-Object {
        $obj = $_ | ConvertFrom-Json
        Write-Host ($obj | ConvertTo-Json -Compress)
    }
    Write-Host "--------------------------------------------------------------------"
    $lineCount = (Get-Content $EventsFile | Where-Object { $_.Trim() }).Count
    Write-Host ""
    Write-Host "Total events captured: $lineCount"

    # -----------------------------------------------------------------------
    # Step 8 — Validate
    # -----------------------------------------------------------------------
    Banner "Step 7 — Validation"
    $lines = Get-Content $EventsFile | Where-Object { $_.Trim() }
    $events = $lines | ForEach-Object { $_ | ConvertFrom-Json }

    $allowedReads  = @($events | Where-Object { $_.status -eq "allowed" -and $_.action -eq "read" })
    $deniedWrites  = @($events | Where-Object { $_.status -eq "denied"  -and $_.action -eq "write" })
    $allowedWrites = @($events | Where-Object { $_.status -eq "allowed" -and $_.action -eq "write" })
    $summaries     = @($events | Where-Object { $_.count -gt 0 })

    Write-Host "  Total events       : $($events.Count)"
    Write-Host "  Allowed reads      : $($allowedReads.Count)"
    Write-Host "  Denied writes      : $($deniedWrites.Count)"
    Write-Host "  Allowed writes     : $($allowedWrites.Count)"
    Write-Host "  Summary events     : $($summaries.Count)"
    Write-Host ""

    $fails = @()
    if ($allowedReads.Count  -eq 0) { $fails += "FAIL: no allowed-read events" }
    if ($deniedWrites.Count  -eq 0) { $fails += "FAIL: no denied-write events" }
    if ($allowedWrites.Count -eq 0) { $fails += "FAIL: no allowed-write events" }
    if ($summaries.Count     -eq 0) { $fails += "FAIL: no summary events (count > 0)" }

    foreach ($f in $fails) { Write-Host $f }

    if ($fails.Count -eq 0) {
        Write-Host "  ALL CHECKS PASSED ✓"
    } else {
        throw "Validation failed: $($fails -join '; ')"
    }

    Banner "Test complete — evidence in $EvidenceDir\"

} finally {
    # Cleanup
    Write-Host ""
    Write-Host "=== Cleanup ==="
    if ($EventsJob) {
        Stop-Job  $EventsJob -ErrorAction SilentlyContinue
        Remove-Job $EventsJob -ErrorAction SilentlyContinue
    }
    if ($Mma2Process -and -not $Mma2Process.HasExited) {
        $Mma2Process.Kill()
    }
    if (Test-Path $Binary) { Remove-Item $Binary -ErrorAction SilentlyContinue }
    Write-Host "Done."
}
