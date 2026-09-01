.PHONY: verify verify-live
verify:
	./scripts/verify.sh

verify-live:
	pwsh -File ./scripts/verify-live.ps1
