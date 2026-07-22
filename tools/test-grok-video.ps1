[CmdletBinding()]
param(
    [string]$BaseUrl = "https://apinode.ltd/v1",
    [string]$ApiKey = $env:APINODE_API_KEY,
    [string]$ImagePath = "",
    [string]$ImageUrl = "",
    [string]$Prompt = "Strictly use the input image as the first frame. Keep the character identity, clothing, body shape, position, and background layout consistent. Create a 10 second single-shot realistic vertical video with natural restrained motion.",
    [string]$Model = "grok-imagine-video-1.5",
    [ValidateSet("480p", "720p", "1080p")]
    [string]$Resolution = "480p",
    [ValidateSet("9:16", "16:9", "1:1")]
    [string]$AspectRatio = "9:16",
    [int]$Duration = 10,
    [int]$PollSeconds = 5,
    [int]$MaxPolls = 60,
    [string]$DownloadTo = ""
)

$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8

function Get-MimeType {
    param([string]$Path)

    $extension = [System.IO.Path]::GetExtension($Path).ToLowerInvariant()
    switch ($extension) {
        ".jpg"  { return "image/jpeg" }
        ".jpeg" { return "image/jpeg" }
        ".png"  { return "image/png" }
        ".webp" { return "image/webp" }
        default { return "application/octet-stream" }
    }
}

function Read-HttpErrorBody {
    param([object]$ErrorRecord)

    if ($ErrorRecord.ErrorDetails -and $ErrorRecord.ErrorDetails.Message) {
        return $ErrorRecord.ErrorDetails.Message
    }

    $response = $ErrorRecord.Exception.Response
    if ($response -and $response.GetResponseStream) {
        try {
            $reader = [System.IO.StreamReader]::new($response.GetResponseStream(), [System.Text.Encoding]::UTF8)
            return $reader.ReadToEnd()
        }
        catch {
            return $ErrorRecord.Exception.Message
        }
    }

    return $ErrorRecord.Exception.Message
}

function Invoke-JsonApi {
    param(
        [string]$Method,
        [string]$Uri,
        [object]$Body = $null
    )

    $headers = @{
        "Authorization" = "Bearer $ApiKey"
        "Accept"        = "application/json"
    }

    try {
        if ($null -ne $Body) {
            $json = $Body | ConvertTo-Json -Depth 20 -Compress
            $bytes = [System.Text.Encoding]::UTF8.GetBytes($json)
            return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers -ContentType "application/json; charset=utf-8" -Body $bytes
        }

        return Invoke-RestMethod -Method $Method -Uri $Uri -Headers $headers
    }
    catch {
        $bodyText = Read-HttpErrorBody $_
        Write-Error "HTTP request failed: $Method $Uri`n$bodyText"
    }
}

function First-Text {
    param([object[]]$Values)

    foreach ($value in $Values) {
        $text = [string]$value
        if (-not [string]::IsNullOrWhiteSpace($text)) {
            return $text.Trim()
        }
    }

    return ""
}

if ([string]::IsNullOrWhiteSpace($ApiKey)) {
    throw "Set -ApiKey or APINODE_API_KEY."
}

$BaseUrl = $BaseUrl.TrimEnd("/")
$submitUrl = "$BaseUrl/videos/generations"

if ([string]::IsNullOrWhiteSpace($ImageUrl)) {
    if ([string]::IsNullOrWhiteSpace($ImagePath)) {
        throw "Set -ImagePath or -ImageUrl for image-to-video testing."
    }

    $resolvedImagePath = (Resolve-Path -LiteralPath $ImagePath).Path
    $imageBytes = [System.IO.File]::ReadAllBytes($resolvedImagePath)
    $mimeType = Get-MimeType $resolvedImagePath
    $ImageUrl = "data:$mimeType;base64,$([System.Convert]::ToBase64String($imageBytes))"
}

$requestBody = [ordered]@{
    model        = $Model
    prompt       = $Prompt
    duration     = $Duration
    aspect_ratio = $AspectRatio
    resolution   = $Resolution
    image        = [ordered]@{
        url = $ImageUrl
    }
}

Write-Host "POST $submitUrl"
$submission = Invoke-JsonApi -Method "POST" -Uri $submitUrl -Body $requestBody
$submission | ConvertTo-Json -Depth 20

$requestId = First-Text @(
    $submission.request_id,
    $submission.id,
    $submission.data.request_id,
    $submission.data.id
)

if ([string]::IsNullOrWhiteSpace($requestId)) {
    throw "Video request id was not found in the submission response."
}

$pollUrl = "$BaseUrl/videos/$([System.Uri]::EscapeDataString($requestId))"
Write-Host "Polling $pollUrl"

for ($attempt = 1; $attempt -le $MaxPolls; $attempt++) {
    Start-Sleep -Seconds $PollSeconds

    $poll = Invoke-JsonApi -Method "GET" -Uri $pollUrl
    $status = First-Text @($poll.status, $poll.data.status)
    $progress = First-Text @($poll.progress, $poll.data.progress)

    if ($progress) {
        Write-Host "[$attempt/$MaxPolls] status=$status progress=$progress"
    }
    else {
        Write-Host "[$attempt/$MaxPolls] status=$status"
    }

    if ($status -in @("done", "completed", "succeeded")) {
        $videoUrl = First-Text @(
            $poll.video.url,
            $poll.data.video.url,
            $poll.url,
            $poll.data.url
        )

        $poll | ConvertTo-Json -Depth 20
        Write-Host "Video URL: $videoUrl"

        if (-not [string]::IsNullOrWhiteSpace($DownloadTo) -and -not [string]::IsNullOrWhiteSpace($videoUrl)) {
            Invoke-WebRequest -Uri $videoUrl -OutFile $DownloadTo
            Write-Host "Downloaded: $DownloadTo"
        }

        exit 0
    }

    if ($status -in @("failed", "cancelled", "expired")) {
        $poll | ConvertTo-Json -Depth 20
        exit 1
    }
}

throw "Polling timed out after $($MaxPolls * $PollSeconds) seconds. Request ID: $requestId"
