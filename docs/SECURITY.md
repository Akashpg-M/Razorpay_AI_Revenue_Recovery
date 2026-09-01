# Security

Secrets belong in environment variables and must never enter logs, replay payloads, fixtures, or screenshots. Razorpay webhook HMAC is verified over the raw body. Request sizes and review enums are bounded, unknown review fields are rejected, and CORS allows only the configured frontend origin.

The repository records operator identity but does not implement enterprise authentication or authorization; production deployment must authenticate operators and map trusted identity into the API. Synthetic and Test Mode labels are mandatory. Rotate any credential accidentally exposed in output.
