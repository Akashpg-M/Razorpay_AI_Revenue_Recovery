$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  docker compose up -d --build
  docker compose ps
  Invoke-RestMethod http://localhost:8080/health/ready | ConvertTo-Json -Depth 5
  Invoke-RestMethod http://localhost:8001/health/ready | ConvertTo-Json -Depth 5
  Invoke-RestMethod http://localhost:8080/api/v1/observability | ConvertTo-Json -Depth 8
} finally { Pop-Location }
