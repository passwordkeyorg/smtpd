# mail-ingest-service

High-throughput email receiving + processing service (scaffold).

## Goals
- Accept large volumes of inbound mail reliably
- Fast ACK path (ingress) with durable spool/queue
- Horizontally scalable workers
- Observable (metrics/logging), testable, maintainable

## Quick start

### Requirements
- Go 1.22+

### Run
```bash
make run-smtpd
# in another terminal
make run-worker
```

## Acceptance checklist
- [ ] SMTP ingress can accept connections and persist raw message to spool
- [ ] Ingress ACK path is fast and independent from processing
- [ ] Worker can read spool items and mark complete
- [ ] `make fmt`, `make test` pass

## Security
- Never commit secrets. Use `.env` or secret manager.
