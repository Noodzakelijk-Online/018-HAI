[CmdletBinding()]
param(
    [string]$RootSessionId = '019e7acc-44f2-7c90-a04e-253f6d43df28',
    [string]$SessionsRoot = 'C:\Users\NO\.codex\sessions\2026\08',
    [string]$OutputDirectory = '',
    [int]$TailBytes = 16777216
)

$ErrorActionPreference = 'Stop'

if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $PSScriptRoot '..\docs'
}

if ($TailBytes -lt 1048576) {
    throw 'TailBytes must be at least 1 MiB.'
}
if (-not (Test-Path -LiteralPath $SessionsRoot -PathType Container)) {
    throw "Sessions root does not exist: $SessionsRoot"
}

if (-not ('HaiTranscriptAllocation' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;

public static class HaiTranscriptAllocation
{
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern uint GetCompressedFileSizeW(string fileName, out uint high);

    public static ulong GetAllocatedBytes(string path)
    {
        uint high;
        uint low = GetCompressedFileSizeW(path, out high);
        if (low == 0xffffffff && Marshal.GetLastWin32Error() != 0)
        {
            throw new Win32Exception();
        }
        return ((ulong)high << 32) | low;
    }
}
'@
}

function Get-TailText {
    param([Parameter(Mandatory)][string]$Path)

    $stream = [System.IO.File]::Open(
        $Path,
        [System.IO.FileMode]::Open,
        [System.IO.FileAccess]::Read,
        [System.IO.FileShare]::ReadWrite
    )
    try {
        $take = if ($stream.Length -lt $TailBytes) {
            [int]$stream.Length
        } else {
            $TailBytes
        }
        [void]$stream.Seek(-[int64]$take, [System.IO.SeekOrigin]::End)
        $buffer = [byte[]]::new($take)
        [void]$stream.Read($buffer, 0, $take)
        return [System.Text.Encoding]::UTF8.GetString($buffer)
    } finally {
        $stream.Dispose()
    }
}

function Get-Sha256 {
    param([AllowEmptyString()][string]$Text)

    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [System.Text.Encoding]::UTF8.GetBytes($Text)
        return ([System.BitConverter]::ToString($algorithm.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant()
    } finally {
        $algorithm.Dispose()
    }
}

function Protect-ReportText {
    param([AllowEmptyString()][string]$Text)

    $protected = $Text
    $protected = $protected -replace '(?i)(Authorization:\s*Bearer\s+)[A-Za-z0-9._~+/=-]+', '$1[REDACTED]'
    $protected = $protected -replace '(?i)sk-[A-Za-z0-9_-]{12,}', 'sk-[REDACTED]'
    $protected = $protected -replace '(?i)ghp_[A-Za-z0-9]{12,}', 'ghp_[REDACTED]'
    $protected = $protected -replace '(?is)-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----.*?-----END (?:RSA |EC |OPENSSH )?PRIVATE KEY-----', '[REDACTED PRIVATE KEY]'
    $protectedLines = @($protected -split "`r?`n" | ForEach-Object { $_.TrimEnd() })
    return ($protectedLines -join "`n").Trim()
}

function ConvertTo-CsvField {
    param([AllowEmptyString()][string]$Value)
    return '"' + ($Value -replace '"', '""') + '"'
}

$records = [System.Collections.Generic.List[object]]::new()
$relativeBase = (Split-Path -Parent $SessionsRoot).TrimEnd('\')
$sessionFiles = Get-ChildItem -LiteralPath $SessionsRoot -Recurse -File -Filter '*.jsonl'

foreach ($file in $sessionFiles) {
    $firstLine = Get-Content -LiteralPath $file.FullName -TotalCount 1
    if ($firstLine -notmatch [regex]::Escape($RootSessionId)) {
        continue
    }

    $metadata = $firstLine | ConvertFrom-Json
    $childId = [string]$metadata.payload.id
    $tail = Get-TailText -Path $file.FullName
    $terminalEvents = @(
        $tail -split "`n" |
            Where-Object { $_ -match '"type":"(task_complete|turn_aborted|task_aborted|task_cancelled)"' }
    )

    $terminalStatus = 'nonterminal'
    $terminalReason = ''
    $finalMessage = ''
    for ($index = $terminalEvents.Count - 1; $index -ge 0; $index--) {
        try {
            $event = $terminalEvents[$index] | ConvertFrom-Json
            $eventType = [string]$event.payload.type
            if ($eventType -eq 'task_complete') {
                $terminalStatus = 'completed'
                $finalMessage = [string]$event.payload.last_agent_message
                break
            }
            if ($eventType -in @('turn_aborted', 'task_aborted', 'task_cancelled')) {
                $terminalStatus = 'aborted'
                $terminalReason = [string]$event.payload.reason
                break
            }
        } catch {
            # A partial first line is expected when the tail starts mid-record.
        }
    }

    $patchCallCount = @(
        $tail -split "`n" |
            Where-Object {
                $_ -match '"type":"custom_tool_call"' -and
                $_ -match '(tools\\?\.apply_patch|\*\*\* Begin Patch)'
            }
    ).Count

    $workKind = if ($terminalStatus -ne 'completed') {
        $terminalStatus
    } elseif ($finalMessage -match '(?i)^Stopped\.|preserved and uncommitted|nothing committed or pushed') {
        'partial-report'
    } elseif ($patchCallCount -gt 0) {
        'implementation'
    } else {
        'advisory'
    }

    $relativePath = $file.FullName.Substring($relativeBase.Length).TrimStart('\').Replace('\', '/')
    $allocatedBytes = [HaiTranscriptAllocation]::GetAllocatedBytes($file.FullName)
    $disposition = if ($terminalStatus -eq 'completed') {
        'candidate_after_ledger_commit'
    } else {
        'retain'
    }

    $records.Add([pscustomobject]@{
        child_id = $childId
        session_date = $file.Directory.Name
        nickname = [string]$metadata.payload.agent_nickname
        terminal_status = $terminalStatus
        terminal_reason = $terminalReason
        work_kind = $workKind
        patch_call_count = $patchCallCount
        logical_bytes = [int64]$file.Length
        allocated_bytes = [uint64]$allocatedBytes
        final_message_sha256 = Get-Sha256 -Text $finalMessage
        final_report_preserved = ($terminalStatus -eq 'completed')
        disposition = $disposition
        session_path = $relativePath
        protected_final_message = Protect-ReportText -Text $finalMessage
    })
}

$records = @($records | Sort-Object session_date, child_id)
$completed = @($records | Where-Object terminal_status -eq 'completed')
$retained = @($records | Where-Object terminal_status -ne 'completed')

$csvHeader = @(
    'child_id', 'session_date', 'nickname', 'terminal_status', 'terminal_reason',
    'work_kind', 'patch_call_count', 'logical_bytes', 'allocated_bytes',
    'final_message_sha256', 'final_report_preserved', 'disposition', 'session_path'
)
$csvLines = [System.Collections.Generic.List[string]]::new()
$csvLines.Add(($csvHeader | ForEach-Object { ConvertTo-CsvField $_ }) -join ',')
foreach ($record in $records) {
    $values = foreach ($column in $csvHeader) {
        ConvertTo-CsvField ([string]$record.$column)
    }
    $csvLines.Add($values -join ',')
}

$reportLines = [System.Collections.Generic.List[string]]::new()
$reportLines.Add('# Completed child-agent final reports')
$reportLines.Add('')
$reportLines.Add('This generated archive preserves the canonical terminal report from every')
$reportLines.Add('completed HAI child transcript in the audited August 2026 cohort. Potential')
$reportLines.Add('credential-shaped values are redacted; the manifest retains the SHA-256 of')
$reportLines.Add('the original terminal message. Aborted and nonterminal transcripts are not')
$reportLines.Add('represented as completed work and must be retained.')
$reportLines.Add('')
foreach ($record in $completed) {
    $agentLabel = if ([string]::IsNullOrWhiteSpace($record.nickname)) {
        '(not recorded)'
    } else {
        $record.nickname
    }
    $reportLines.Add("## $($record.child_id)")
    $reportLines.Add('')
    $reportLines.Add("- Date: 2026-08-$($record.session_date)")
    $reportLines.Add("- Agent: $agentLabel")
    $reportLines.Add("- Work kind: $($record.work_kind)")
    $reportLines.Add('- Original report SHA-256: `' + $record.final_message_sha256 + '`')
    $reportLines.Add('- Transcript: `' + $record.session_path + '`')
    $reportLines.Add('')
    if ([string]::IsNullOrWhiteSpace($record.protected_final_message)) {
        $reportLines.Add('_The child completed without a terminal text report._')
    } else {
        $reportLines.Add($record.protected_final_message)
    }
    $reportLines.Add('')
}

$normalizedReportLines = @($reportLines | ForEach-Object { $_.TrimEnd() })
$lastReportLine = $normalizedReportLines.Count - 1
while ($lastReportLine -ge 0 -and $normalizedReportLines[$lastReportLine] -eq '') {
    $lastReportLine--
}
if ($lastReportLine -ge 0) {
    $normalizedReportLines = @($normalizedReportLines[0..$lastReportLine])
}

$summary = [ordered]@{
    generated_at_utc = [DateTime]::UtcNow.ToString('o')
    root_session_id = $RootSessionId
    audited_transcripts = $records.Count
    completed_transcripts = $completed.Count
    retained_transcripts = $retained.Count
    completed_logical_bytes = [int64](($completed | Measure-Object logical_bytes -Sum).Sum)
    completed_allocated_bytes = [uint64](($completed | Measure-Object allocated_bytes -Sum).Sum)
    retained_logical_bytes = [int64](($retained | Measure-Object logical_bytes -Sum).Sum)
    retained_allocated_bytes = [uint64](($retained | Measure-Object allocated_bytes -Sum).Sum)
    completed_reports_file = 'child-agent-final-reports.md'
    manifest_file = 'child-agent-transcript-manifest.csv'
    deletion_performed = $false
}

[System.IO.Directory]::CreateDirectory($OutputDirectory) | Out-Null
$utf8 = [System.Text.UTF8Encoding]::new($false)
[System.IO.File]::WriteAllLines(
    (Join-Path $OutputDirectory 'child-agent-transcript-manifest.csv'),
    $csvLines,
    $utf8
)
[System.IO.File]::WriteAllLines(
    (Join-Path $OutputDirectory 'child-agent-final-reports.md'),
    $normalizedReportLines,
    $utf8
)
[System.IO.File]::WriteAllText(
    (Join-Path $OutputDirectory 'child-agent-transcript-summary.json'),
    ($summary | ConvertTo-Json -Depth 3),
    $utf8
)

$summary | ConvertTo-Json -Depth 3
