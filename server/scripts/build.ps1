param()
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot/..
New-Item -ItemType Directory -Path "bin" -Force | Out-Null
go build -o bin/remote-server ./cmd/server
Write-Host "Build output: server/bin/remote-server"

