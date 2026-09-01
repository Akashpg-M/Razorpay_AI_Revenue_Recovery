$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$results = [ordered]@{}
function Step([string]$name, [scriptblock]$command) {
  try { & $command; if ($LASTEXITCODE -ne 0) { throw "$name exited $LASTEXITCODE" }; $results[$name] = "PASS" }
  catch { $results[$name] = "FAIL"; throw }
}
try {
  Push-Location "$root/backend"
  $env:GOCACHE = "$root/.cache/go-build"
  Step "go_format" { $bad = gofmt -l .; if ($bad) { Write-Error "Unformatted Go files: $bad" } }
  Step "go_vet" { go vet ./... }
  Step "go_test" { go test ./... }
  Step "go_build" { go build ./cmd/api ./cmd/worker ./cmd/evaluation }
  Pop-Location
  # Python model artifacts require the exact locked scikit-learn environment;
  # frontend verification likewise uses the repository's pinned Node image.
  Push-Location $root
  Step "python_tests" { docker compose run --rm -v "$root/decision-service:/app" decision-service python -m unittest discover -s tests -p "test_*.py" }
  Step "frontend_image" { docker compose build frontend }
  Step "frontend_lint_build" { docker compose run --rm frontend sh -c "npm run lint && npm run build" }
  Pop-Location
} finally {
  while ((Get-Location).Path -ne $root) { Pop-Location }
  $status = if ($results.Values -contains "FAIL") { "FAIL" } else { "PASS" }
  $artifact = [ordered]@{ schema_version="verification-summary-v1"; generated_at=(Get-Date).ToUniversalTime().ToString("o"); status=$status; checks=$results; invariant_groups=@("domain_state_machine","ml_data_integrity","eligibility_optimizer","economic_policy_safety","workflow_reliability","promise_to_pay","attribution","human_review") }
  New-Item -ItemType Directory -Force "$root/evaluation/results/phase33" | Out-Null
  $artifact | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 "$root/evaluation/results/phase33/verification_summary.json"
}
