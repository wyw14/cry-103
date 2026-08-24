# CableGuard

CableGuard is a control service for submarine cable span monitoring, landing
station feed isolation, protection switching, and repair acceptance.

Build the service with `go build -mod=vendor ./cmd/cableguard` and run it with
`cableguard -listen 127.0.0.1:19703 -data ./data`. The process exposes a health
endpoint and operational APIs under `/api`.
