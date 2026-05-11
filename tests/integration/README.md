# Integration Connectivity Test

This test verifies that `api-gateway` and `matcher` are deployed on the same Docker network and that key endpoints are reachable.

## Run

```sh
docker compose up --abort-on-container-exit integration-test
```

The test checks:
- `matcher` health on `http://matcher:8081/healthz`
- `matcher` readiness on `http://matcher:8081/readyz`
- `api-gateway` transactions endpoint on `http://api:8000/api/transactions`
- `api-gateway` incidents endpoint on `http://api:8000/api/incidents`
- `api-gateway` metrics endpoint on `http://api:8000/api/metrics/mismatch-per-minute`
- optionally, a transaction detail lookup via `http://api:8000/api/transactions/{tx_id}` if a transaction exists

## Report output

The integration test writes:
- `tests/integration/test_report.txt`
- `tests/integration/test_report.json`

These files contain the full case-by-case result summary and final pass/fail counts.

## Notes

- `postgres_api` listens on host port `5432`
- `matcher_postgres` listens on host port `5433`
- `redis` listens on host port `6379`
- `api-gateway` listens on host port `8000`
- `matcher` listens on host port `8081`
