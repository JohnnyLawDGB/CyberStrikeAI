# digiscope.me — security assessment & publish bundle (2026-08-09)

Run: CyberStrikeAI v1.7.12, deep multi-agent, Claude Opus 4.8. Non-destructive (≤30 req/s).
Result: **STRONG / B+ — no critical or high-severity vulnerabilities.**

## Files

| File | What it is |
|---|---|
| `digiscope-full-technical-report.md` | **The findings.** Full technical report: exec summary, attack-surface inventory, 11 findings (severity/evidence/impact/remediation), verified non-issues, prioritized roadmap. Internal — not for public posting. |
| `security-page.html` | **Public, sanitized** security-posture page for `https://digiscope.me/security`. Positive posture only; omits the open-resolver/CSP/token/SSH/version details. Contact: mark@aroundtheblock.us. |
| `security.txt` | RFC 9116 disclosure file for `https://digiscope.me/.well-known/security.txt`. |
| `remediation/nginx-security.conf` | Drop-in nginx hardening: strict CSP (report-only first), HSTS+preload, COEP/Permissions/X-Permitted headers, www→apex 301, and serving the two files above. Covers FIND-02/03/04/06 + publish. |

## Findings → where the fix lives (for the repo to route)

| ID | Sev | Fix location |
|---|---|---|
| FIND-01 open recursive DNS resolver :53 | **Medium** | HOST/firewall — `recursion no` / `allow-recursion { localhost; }` + RRL, or block inbound 53 udp+tcp. Google Cloud DNS is authoritative, so origin needs no public DNS. |
| FIND-02 weak CSP | Medium (DoD) | nginx (`remediation/nginx-security.conf`, report-only → enforce) + Helmet on API |
| FIND-03 missing COEP/Permissions/X-Permitted | Low | nginx / Helmet |
| FIND-04 HSTS no preload | Low | nginx |
| FIND-05 session JWT in localStorage | Low | APP CODE — move to HttpOnly/Secure/SameSite cookie (+ CSRF), or strict CSP + short TTL |
| FIND-06 www not 301'd to apex | Low | nginx |
| FIND-07 verbose JSON parse error | Info | APP CODE — generic `{"error":"Invalid JSON body"}` |
| FIND-08 sensitive routes in robots.txt | Info | ensure server-side authz on those routes |
| FIND-09 wallet APK no integrity signal | Info | publish `sha256sum` (+ PGP) next to the .apk |
| FIND-10 SSH SHA-1 HMACs | Info/Low | HOST sshd — `MACs hmac-sha2-512-etm@…,hmac-sha2-256-etm@…` |
| FIND-11 401 uniformity / DMARC rua-ruf / CAA / security.txt | Info | app (401) · DNS (CAA `0 issue "letsencrypt.org"`, DMARC rua/ruf) · security.txt (included) |
