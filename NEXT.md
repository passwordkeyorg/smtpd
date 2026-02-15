# NEXT: mail-ingest-service (world-class SMTP)

This file is a running plan + review log.

## Recently shipped (2026-02-15)

- **security**: Require `METRICS_LISTEN` to be localhost only (reject `:9090` / `0.0.0.0:xxxx`).
  - Commit: `ca15abb`
- **observability**: Track Kafka async publish completions + errors via `kafka-go` writer completion hook.
  - Metrics:
    - `mail_ingest_kafka_published_total`
    - `mail_ingest_kafka_publish_errors_total`
  - Commit: `2bbc5d9`

## Gaps / improvements

### Reliability contract (SMTP semantics)
- Ensure **250 means durably stored** (already true for spool; document + tests).
- Add explicit **spool fsync/atomic write** guarantees (verify spool implementation).
- Add **duplicate suppression** / idempotency strategy for retries.

### Storage (Ceph RGW)
- Treat Ceph RGW as the only object store (S3 compatible): document required env + credentials + bucket lifecycle.
- Add operational checks/alerts for RGW reachability and upload error rate.

### Indexer (world-class line)
- Define a replayable ingest log (spool + optional Kafka): every message gets a stable id, sha256, and immutable meta.
- Make indexer **idempotent** and **restartable** (checkpointing), with clear failure quarantine.
- Metrics/SLO: lag (backlog age), throughput, error rates; alert when lag grows.

### Backpressure + overload safety
- When spool disk is near-full, return 452 early (avoid thrash).
- Add per-IP + per-domain connection/command limits; expose metrics.

### Observability & SLOs
- Add histogram: spool store latency.
- Add gauge: spool queue depth (pending meta/eml files).
- Add metric + alert: disk free bytes / percent.

### Kafka integration (best-effort but visible)
- Add alert rule: any increase in `mail_ingest_kafka_publish_errors_total` over 5m.
- Consider a bounded in-process queue for Kafka publish to avoid goroutine explosion under load.

### User inbox (web)
- Build a lightweight web inbox for listing/searching messages and downloading raw EML.
- Not a Gmail-style full email client.

### Security / abuse resistance
- Slowloris defenses (command timeouts, per-connection rate limits).
- Recipient validation behavior and anti-enumeration options.

## Product route (single platform, 3 plans)

One unified core model (Tenant/Domain/Mailbox/Access/EventLog) with 3 plans:

Core principle: **Event Log as a first-class product feature** (replayable, queryable), not just internal logs.

- **Developer / API-first**: Token/API key + Webhook; UI optional.
- **Temporary inbox**: Random address as credential + simple web inbox; strict anti-enumeration + short TTL.
- **Enterprise**: Custom domains + API key + Webhook + (later) SSO/OIDC; IP allowlist for control-plane; audit logs.

## Next concrete steps (ordered)

1) **Access model + scopes**
   - Define token/key model **scoped to concrete resources** (tenant → domain → mailbox) with explicit allow-lists.
     - e.g. token can read *only* `domain=A` and `mailbox=foo`, not global.
   - Admin endpoints to mint/revoke keys.

2) **Webhook v1 (event schema + delivery contract)**
   - Keep webhook payloads **lightweight**: no full message body; include ids + pointers so clients pull via API.
   - Include `trace_id` on every event; carry it end-to-end for audit/debug.
   - Event types: `ingest.received`, `objectstore.uploaded`, `indexer.indexed`, `message.quarantined`, `webhook.delivered`.
   - Delivery: retries + signature + idempotency key.

2.5) **Web Inbox MVP auth + download security**
   - Support both Magic Link and SSO(OIDC) (plan defaults: Dev=Magic Link, Temp=inbox_token URL, Enterprise=SSO + Magic Link fallback).
   - Magic Link anti-enumeration: always 200 + similar timing (async send queue / fixed delay).
   - Token binding: no IP bind; optional coarse UA class bind.
   - Rate limit: dual-key (per email + per IP; either triggers deny).
   - CSRF: cookie-based web writes require CSRF; API auth uses Authorization header only (no cookies).
   - Magic Link requires separate outbound email channel (SES/Postmark/SendGrid) isolated from ingest.
   - Raw EML download via short-lived signed URL / one-time download token (5-10 min TTL).
   - MVP search UX: single search box with backend query typing.

3) **Indexer world-class line (checkpoint + replay + quarantine)**
   - Checkpoint store + lag metrics + quarantine path for parse failures.

3.5) **Domain verification + MX monitoring (two-layer policy)**
   - MX must exist and point to us; missing MX is unhealthy (RFC 5321 fallback to A/AAAA).
   - Soft reject (451) while TXT still verified; hard reject (550) after 48h or TXT missing; mark suspended.
   - Emit `domain.mx_lost` / `domain.suspended` events and notify.

4) **Ops: alerts + SLO metrics**
   - Kafka publish errors > 0
   - Spool errors rate > 0
   - Disk free < threshold
   - Indexer lag age growing

5) Add spool queue depth metric (and corresponding alert).

6) Add spool store latency histogram.

7) Audit spool persistence semantics (atomic rename + fsync) and write tests.
