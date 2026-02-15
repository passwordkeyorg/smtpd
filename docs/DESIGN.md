# Design notes (living)

## Scope model (Access)

Goal: scopes bind to *concrete resources*, not just global roles.

Hierarchy:

- Tenant
  - Domain
    - Mailbox
      - Message

Recommendation:

- Keys/tokens carry an allow-list of resource selectors:
  - tenant_id (always)
  - domain_ids (optional)
  - mailbox_ids (optional)
  - actions: read/search/download_raw/manage_webhooks/manage_domains/manage_mailboxes

Rules:

- If domain_ids omitted => all domains in tenant.
- If mailbox_ids omitted => all mailboxes in the selected domains.
- Server enforces scope at query time (SQL WHERE / index filter), not just at routing.

## Auth (Web Inbox)

We support both Magic Link and SSO (OIDC), with plan defaults:

- Developer / Temp plans: Magic Link by default
- Enterprise: SSO preferred, Magic Link as fallback

### Magic Link

- One-time token
- TTL: 15 minutes
- **Do not IP-bind** (users often request on phone, click on desktop; enterprise VPN/proxy changes IP). 
- Optional **coarse UA binding** only (e.g. platform class: mobile/desktop/tablet) to reduce replay risk.
- Exchange token for session cookie
- Anti-enumeration: request endpoint always returns 200 and should have similar response timing.
  - Implementation option: always create+store token; only enqueue provider send for known accounts.
  - If provider send adds latency, use async queue and/or add fixed delay jitter to equalize.

Operational note: mail-ingest is inbound-only; sending Magic Links requires a **separate outbound email channel** (SES/Postmark/SendGrid), strictly isolated from the ingest pipeline.

### Rate limits (must be dual-key)

Apply *both* limits; either can deny:

- per email: e.g. 1/min, 5/hour
- per IP: e.g. 20/hour

### CSRF separation

Do not mix cookie-auth web endpoints with API key/bearer endpoints.

- Web Inbox (cookie): write actions require CSRF token.
- API (Authorization header): do **not** read cookies; CSRF not applicable.

## Webhook v1

Principle: webhook payloads must be lightweight and bounded.

Payload includes:

- event_id
- event_type
- occurred_at
- trace_id
- tenant_id, domain, mailbox
- message_id, sha256, bytes
- API pointers:
  - message_url
  - raw_eml_url

No full body/attachments in webhook.

Delivery contract:

- signed requests (HMAC)
- retries with exponential backoff
- idempotency key = event_id
- delivery receipts recorded as events: `webhook.delivered`

## Search (Web Inbox MVP)

MVP UX: a single search box (not a matrix of filters). Backend interprets query:

- Looks like an email address -> search by `from`
- Looks like SHA256 -> exact match
- Looks like Message-ID (`<...>`) -> exact match
- Otherwise -> subject text match

Advanced filters can come later.

## Raw EML download security

Do not expose predictable permanent URLs.

- Prefer **short-lived signed URLs** (S3-presigned style) for raw EML download (5-10 minutes TTL).
- Or require a short-lived download token scoped to message_id + action.
- For Temp plan: prefer inbox_token URLs (address+secret) and avoid outbound Magic Link completely.

## Event Log (killer feature)

Every message gets a trace_id. We emit events through the pipeline:

- smtp.session_started
- smtp.mail_from
- smtp.rcpt_to
- ingest.received (spool persisted)
- objectstore.uploaded
- indexer.indexed
- policy.quarantined (if any)
- webhook.attempted / webhook.delivered

Event log must be:

- replayable
- queryable by trace_id / message_id / domain / mailbox
- exportable for compliance

## Domain verification

Design for the ugly parts early.

Domain fields (minimum):

- verification_status: pending|verified|failed
- verification_method: dns_txt
- verification_token
- mx_status: ok|missing|wrong_target
- mx_lost_since (nullable timestamp)
- suspended: bool
- suspended_reason
- last_checked_at
- last_error

MX policy (important):

- **MX must exist** and at least one record must point to our ingress.
- If MX is missing, RFC 5321 allows senders to fall back to A/AAAA delivery.
  That means mail may be delivered elsewhere without us seeing it. So "no MX" is treated as unhealthy.

Two-layer handling strategy:

- Layer 1: MX lost but TXT verification still present (likely user mistake)
  - Soft reject at RCPT: `451 4.7.0 Domain temporarily unavailable`
  - Emit event: `domain.mx_lost`
  - Notify via webhook/contact email if configured

- Layer 2: MX lost > 48h OR TXT verification also missing (likely domain abandoned)
  - Hard reject at RCPT: `550 5.1.2 Domain not available`
  - Mark domain as `suspended=true` with reason
  - Emit event: `domain.suspended`

Flow:

1) User adds TXT record containing verification_token
2) We periodically check DNS until verified
3) After verified, guide MX record configuration (priority, multiple MX)
4) Continuously monitor TXT+MX; apply the two-layer policy above
5) Optionally: SPF/DKIM alignment guidance (future)
