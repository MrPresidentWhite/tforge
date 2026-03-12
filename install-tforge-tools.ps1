param(
    # Zielverzeichnis für die Binaries, standard: $HOME\.tforge\bin
    [string]$InstallDir = "$HOME\.tforge\bin"
)

Write-Host "Installiere tforge CLI und Agent nach $InstallDir`n"

# Projektwurzel annehmen: dieses Script aus dem Repo-Root starten
$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $RepoRoot

# Sicherstellen, dass der Ordner existiert
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir | Out-Null
}

# 1. tforge-agent bauen
Write-Host "Baue tforge-agent..."
go build -o (Join-Path $InstallDir "tforge-agent.exe") ./cmd/tforge-agent
if ($LASTEXITCODE -ne 0) {
    Write-Error "Fehler beim Bauen von tforge-agent"
    exit 1
}

# 2. tforge CLI bauen
Write-Host "Baue tforge..."
go build -o (Join-Path $InstallDir "tforge.exe") ./cmd/tforge
if ($LASTEXITCODE -ne 0) {
    Write-Error "Fehler beim Bauen von tforge"
    exit 1
}

# 3. Sicherstellen, dass InstallDir im PATH ist (für aktuelle Session und Benutzer)
$installDirFull = [IO.Path]::GetFullPath($InstallDir)
$pathEntries = $env:PATH -split ';' | Where-Object { $_ -ne '' }

if ($pathEntries -notcontains $installDirFull) {
    Write-Host "`nFüge $installDirFull temporär zum PATH dieser PowerShell-Session hinzu..."
    $env:PATH = "$installDirFull;$env:PATH"
} else {
    Write-Host "`n$installDirFull ist bereits im PATH (Session)."
}

# Persistenter Benutzer-PATH
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
$userEntries = $userPath -split ';' | Where-Object { $_ -ne '' }
if ($userEntries -notcontains $installDirFull) {
    Write-Host "Trage $installDirFull dauerhaft in den Benutzer-PATH ein..."
    $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
        $installDirFull
    } else {
        "$installDirFull;$userPath"
    }
    [Environment]::SetEnvironmentVariable("PATH", $newUserPath, "User")
} else {
    Write-Host "$installDirFull ist bereits im Benutzer-PATH hinterlegt."
}


# 4. Autostart-Eintrag für tforge-agent im Benutzer-Startup-Ordner anlegen
try {
    $startupDir = Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs\Startup'
    if (Test-Path $startupDir) {
        $shortcutPath = Join-Path $startupDir 'tforge-agent.lnk'
        $wshShell = New-Object -ComObject WScript.Shell
        $shortcut = $wshShell.CreateShortcut($shortcutPath)
        $shortcut.TargetPath = Join-Path $installDirFull 'tforge-agent.exe'
        $shortcut.WorkingDirectory = $installDirFull
        # minimiert starten, damit kein Konsolenfenster im Vordergrund bleibt
        $shortcut.WindowStyle = 7
        $shortcut.Description = 'TForge Agent – lokaler Secrets-Dienst'
        $shortcut.Save()

        Write-Host "`nAutostart für tforge-agent wurde im Benutzer-Startup-Ordner eingerichtet."
        Write-Host "Der Agent startet nun automatisch im Hintergrund, wenn du dich bei Windows anmeldest."
        Write-Host "Zum Deaktivieren kannst du die Verknüpfung 'tforge-agent.lnk' im Startup-Ordner löschen."
    } else {
        Write-Warning "Konnte den Startup-Ordner nicht finden, Autostart für tforge-agent wurde nicht eingerichtet."
    }
}
catch {
    Write-Warning "Fehler beim Einrichten des Autostarts für tforge-agent: $_"
}

Write-Host "`nFertig."
Write-Host "Du kannst tforge nun z.B. so verwenden (in einem neuen Terminal):"
Write-Host "  tforge @CineVault -- npm run dev"