@echo off
setlocal
rem Stop the app first. DATA_DIR is resolved like the app's setting.
set "FREECOMPARE_ROOT=%~dp0"
powershell.exe -NoProfile -NonInteractive -Command "$ErrorActionPreference = 'Stop'; $dataPath = if ([string]::IsNullOrEmpty($env:DATA_DIR)) { Join-Path $env:FREECOMPARE_ROOT 'data' } else { $env:DATA_DIR }; $dataPath = [IO.Path]::GetFullPath($dataPath); if ($dataPath.TrimEnd('\','/') -eq [IO.Path]::GetPathRoot($dataPath).TrimEnd('\','/')) { throw 'Refusing to clean a filesystem root.' }; $uploadsPath = [IO.Path]::GetFullPath((Join-Path $dataPath 'uploads')); $prefix = $dataPath.TrimEnd('\','/') + [IO.Path]::DirectorySeparatorChar; if (-not $uploadsPath.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Uploads path is outside the data directory.' }; foreach ($name in @('bracket.db','bracket.db-wal','bracket.db-shm','bracket.db-journal')) { $filePath = Join-Path $dataPath $name; if (Test-Path -LiteralPath $filePath) { Remove-Item -LiteralPath $filePath -Force } }; if (Test-Path -LiteralPath $uploadsPath) { Remove-Item -LiteralPath $uploadsPath -Recurse -Force }; Write-Host ('Reset app database and uploads in ' + $dataPath)"
exit /b %ERRORLEVEL%
