param(
    # Optionales Zielverzeichnis für die kopierte EXE (Standard: helper-bin im Repo-Root)
    [string]$OutDir = ""
)

$ErrorActionPreference = "Stop"

# Projekt-Root bestimmen (Verzeichnis dieses Scripts)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot  = $ScriptDir

# Standard-Ziel: helper-bin im Repo-Root
if ([string]::IsNullOrWhiteSpace($OutDir)) {
    $OutDir = Join-Path $RepoRoot "helper-bin"
}

Write-Host "Repo-Root: $RepoRoot"
Write-Host "Zielverzeichnis für Helper-Binaries: $OutDir"
Write-Host ""

# Pfad zum C#-Projekt
$HelperProjDir = Join-Path $RepoRoot "hello-helper\TForge.HelloHelper"
$CsprojPath    = Join-Path $HelperProjDir "TForge.HelloHelper.csproj"

if (-not (Test-Path $CsprojPath)) {
    Write-Error "C#-Projekt wurde nicht gefunden: $CsprojPath"
    exit 1
}

if (-not (Get-Command dotnet -ErrorAction SilentlyContinue)) {
    Write-Error "Das .NET SDK ('dotnet') wurde nicht im PATH gefunden. Installiere es von https://dotnet.microsoft.com/download"
    exit 1
}

# Publish in ein eigenes, temporäres Verzeichnis. Der Ausgabepfad wird explizit
# vorgegeben, damit das Script nicht vom Target-Framework des csproj abhängt --
# eine frühere Version hatte den Pfad fest auf net6.0 verdrahtet und fand die
# EXE nach dem Wechsel auf net8.0 nicht mehr.
$PublishDir = Join-Path $HelperProjDir "bin\publish-helper"

if (Test-Path $PublishDir) {
    Remove-Item -Recurse -Force $PublishDir
}

Write-Host "Publish TForge.HelloHelper (Release, SingleFile, self-contained)..."
Push-Location $HelperProjDir
try {
    dotnet publish -c Release -o $PublishDir
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Publish von TForge.HelloHelper ist fehlgeschlagen (ExitCode $LASTEXITCODE)."
        exit 1
    }
} finally {
    Pop-Location
}

Write-Host ""
Write-Host "Publish erfolgreich. Suche nach SingleFile-EXE..."

$exePath = Join-Path $PublishDir "TForge.HelloHelper.exe"

if (-not (Test-Path $exePath)) {
    Write-Error "Konnte SingleFile-EXE nicht finden: $exePath"
    exit 1
}

Write-Host "Gefundene Helper-EXE (SingleFile): $exePath"

# Zielverzeichnis anlegen
if (-not (Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir | Out-Null
}

# Zieldateiname im Zielverzeichnis (für Go-Integration)
$targetExe = Join-Path $OutDir "tforge-hello-helper.exe"

Write-Host "Kopiere Helper-EXE (SingleFile) nach: $targetExe"
Copy-Item -Path $exePath -Destination $targetExe -Force

Write-Host ""
Write-Host "Fertig. Windows Hello Helper steht nun hier bereit:"
Write-Host "  $targetExe"
