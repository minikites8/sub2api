[CmdletBinding()]
param(
    [string]$ApiKey = $env:XAI_API_KEY,
    [string]$ImagePath = "",
    [string]$ImageUrl = "",
    [string]$Prompt = "Generate a slow and serene time-lapse",
    [string]$Model = "grok-imagine-video-1.5",
    [ValidateSet("480p", "720p", "1080p")]
    [string]$Resolution = "480p",
    [ValidateSet("9:16", "16:9", "1:1")]
    [string]$AspectRatio = "16:9",
    [int]$Duration = 12,
    [int]$PollSeconds = 5,
    [int]$MaxPolls = 60,
    [string]$DownloadTo = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

if ([string]::IsNullOrWhiteSpace($ApiKey)) {
    throw "Set -ApiKey or XAI_API_KEY."
}

$scriptPath = Join-Path $PSScriptRoot "test-grok-video.ps1"
& $scriptPath `
    -BaseUrl "https://api.x.ai/v1" `
    -ApiKey $ApiKey `
    -ImagePath $ImagePath `
    -ImageUrl $ImageUrl `
    -Prompt $Prompt `
    -Model $Model `
    -Resolution $Resolution `
    -AspectRatio $AspectRatio `
    -Duration $Duration `
    -PollSeconds $PollSeconds `
    -MaxPolls $MaxPolls `
    -DownloadTo $DownloadTo
