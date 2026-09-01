$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  docker compose up -d --build
  Get-Content -Raw "$root/infra/demo_seed.sql" | docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
  Invoke-RestMethod http://localhost:8080/health/ready | ConvertTo-Json -Depth 5
  Write-Host "Demo ready: http://localhost:3000/operations"
  Write-Host "Replay: http://localhost:3000/recovery/demo-recovered-case-v1"
} finally { Pop-Location }
