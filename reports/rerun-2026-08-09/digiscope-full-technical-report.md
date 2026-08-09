> **Note on record-keeping:** This conversation is not bound to a project, so the project blackboard (`upsert_project_fact`) and vulnerability-store writes were unavailable to me. All evidence is retained inline below. (Sub-agents referenced local vulnerability IDs from their isolated contexts; those are not persisted to a shared project store here.)

---

# FINAL SECURITY ASSESSMENT REPORT — digiscope.me

**Engagement:** Authorized, non-destructive security assessment (passive + safe-active)
**In-scope:** `digiscope.me`, `www.digiscope.me`, `api.digiscope.me` (all resolve to `134.199.198.90`)
**ROE compliance:** ✅ Non-destructive only; no exploitation, no data modification, no config changes, no DoS; scan rates held ≤30 req/s (observed peaks ~27 RPS); no brute forcing.

---

## 1. Executive Summary

DigiScope is a **static React/Vite single-page application** (a DigiByte blockchain explorer + wallet + "Enigma" AI oracle + rewards platform) fronted by **nginx** on a single **DigitalOcean** origin, with a **Node.js/Express + Helmet.js** backend API. The overall security posture is **strong and well-engineered** — noticeably above average for a young production application.

Across full-chain reconnaissance, network/service scanning, TLS analysis, web-app testing, automated vulnerability scanning (nuclei), and a manual API deep-dive, **no critical or high-severity vulnerabilities were identified.** The application demonstrates deliberate hardening: A-grade TLS, strict DMARC/SPF, suppressed version banners, blocked dotfiles, no directory listing, no source-map/backup exposure, correct authorization enforcement, a strict allowlist CORS policy, and global rate limiting.

The one **medium** issue is at the infrastructure layer: an **open recursive DNS resolver** on port 53, usable as a DDoS-amplification vector against third parties. Remaining findings are **defense-in-depth hardening items** (weak CSP, session token in localStorage, missing COEP/Permissions-Policy, www canonicalization, HSTS preload, legacy SSH HMACs).

**Overall security-posture rating: STRONG / GOOD (B+).** One medium infra fix + a handful of hardening improvements would raise it to excellent.

---

## 2. Attack-Surface Inventory

| Attribute | Detail |
|---|---|
| **Origin IP** | `134.199.198.90` — DigitalOcean LLC, ASN 14061, CIDR 134.199.128.0/17 |
| **Hosts** | `digiscope.me` (apex), `www.digiscope.me` (CNAME→apex), `api.digiscope.me` — all → same IP |
| **CDN/WAF** | None — origin directly exposed |
| **DNS** | Google Cloud DNS (ns-cloud-a1..a4.googledomains.com); **DNSSEC unsigned**; **no CAA**; no wildcard; no PTR |
| **Registrar** | Squarespace Domains LLC; created 2026-01-03 (young domain) |
| **Subdomains** | Only `www` and `api` (subfinder + amass + CT logs all agree) |
| **Email** | No MX; SPF `v=spf1 -all`; DMARC `p=reject; sp=reject; adkim=s; aspf=s`; no DKIM |

**Open ports on 134.199.198.90:**

| Port | Service | Version | Notes |
|---|---|---|---|
| 22/tcp | SSH | OpenSSH 9.6p1 (Ubuntu 24.04, 3ubuntu13.18) | Current, SSHv2 only; offers legacy SHA1 HMACs |
| 53/tcp | DNS | (version suppressed) | **Open recursive resolver** — see FIND-01 |
| 80/tcp | HTTP | nginx (server_tokens off) | 301 → HTTPS |
| 443/tcp | HTTPS | nginx (server_tokens off) | TLS 1.2/1.3, A-grade |

Filtered/closed (not exposed): 21, 25, 110, 143, 465, 587, 993, 995, 3306, 5432, 6379, 27017, 8000, 8080, 8443 — **no databases, mail, or admin ports exposed.**

**Technology stack:**
- **Frontend:** React SPA (Vite build); libs: framer-motion, recharts, PixiJS, Lucide; assets under `/_static/` (immutable, 1-yr cache).
- **Backend:** Node.js/Express + Helmet.js; JSON API; global rate limiting (express-rate-limit, 500 req / 900 s / IP).
- **Auth:** Digi-ID login → **JWT Bearer** (stored in `localStorage['dgb_session_token']`); WebAuthn/passkey via `/c/*`.
- **Proxied backend prefixes** (via both web + api origins): `/address/*` (DGB address validator/explorer), `/u/*` (user profiles), `/c/*` (WebAuthn credentials), `/api/address/{addr}/generate` (Enigma AI, Bearer-protected, 10/day quota).

**TLS:** TLS 1.2 + 1.3 only (no SSLv3/1.0/1.1); AEAD ciphers only (ECDHE-ECDSA AES-GCM + ChaCha20-Poly1305), forward secrecy, no weak/RC4/3DES/CBC. Let's Encrypt ECDSA P-256 certs, valid 2026-07-03 → 2026-10-01; correct SNI per vhost. HSTS present on all hosts (no `preload`).

---

## 3. Findings

### FIND-01 — Open Public Recursive DNS Resolver — **MEDIUM**
- **Asset:** `134.199.198.90:53/tcp+udp`
- **Evidence:**
  ```
  dig @134.199.198.90 google.com A +short   → 142.250.217.110
  dig @134.199.198.90 example.com A         → ;; flags: qr rd ra   (ra = recursion available)
  dig @134.199.198.90 . NS +short           → full root-server set (amplification potential)
  ```
- **Impact:** The server answers recursive queries for arbitrary external domains from any source IP. It can be abused as a **DNS amplification/reflection DDoS** vector against third parties (harming the host's IP reputation and bandwidth) and is exposed to cache-poisoning risk.
- **Remediation:** Disable recursion for public clients (`recursion no` / `allow-recursion { localhost; internal; }`), or firewall UDP/TCP 53 from the Internet if DNS need not be public. Enable Response Rate Limiting (RRL). If this port is unintentional, close it.

### FIND-02 — Weak Content-Security-Policy — **MEDIUM (defense-in-depth)**
- **Asset:** `digiscope.me`, `www.digiscope.me` (HTML); CSP also **absent** on `api.digiscope.me` responses.
- **Evidence:** `Content-Security-Policy: frame-ancestors 'self'` only — no `default-src`/`script-src`/`object-src`/`base-uri`. Corroborated by nuclei `weak-csp-detect: missing-script-src / missing-object-src`. API responses omit CSP entirely.
- **Impact:** CSP currently provides only clickjacking protection. It offers **no mitigation against XSS/script injection** — meaningful for a wallet/crypto app rendering address data. This amplifies the localStorage-token risk (FIND-05).
- **Remediation:** Deploy a strict policy (report-only first), e.g. `default-src 'self'; script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'self'; connect-src 'self' https://api.digiscope.me; img-src 'self' data:`. Apply to API HTML responses too.

### FIND-03 — Missing COEP / Permissions-Policy / X-Permitted-Cross-Domain-Policies — **LOW**
- **Asset:** all three hosts (web + api)
- **Evidence:** nuclei `http-missing-security-headers` — COEP and X-Permitted-Cross-Domain-Policies absent; Permissions-Policy absent on API responses (present on web app: `camera=(), microphone=(), geolocation=()`).
- **Impact:** Full cross-origin isolation not achieved (COOP+CORP set, COEP missing); minor hardening gap.
- **Remediation:** Add `Cross-Origin-Embedder-Policy: require-corp` (if compatible), `X-Permitted-Cross-Domain-Policies: none`, and a `Permissions-Policy` on API responses.

### FIND-04 — HSTS without `preload` (and inconsistent max-age) — **LOW**
- **Asset:** web (`max-age=31536000; includeSubDomains`), api (`max-age=15552000; includeSubDomains`) — neither has `preload`.
- **Impact:** First-visit TLS-stripping window remains; inconsistent policy across hosts.
- **Remediation:** Add `preload`, standardize max-age to ≥1 year, submit to hstspreload.org.

### FIND-05 — Session JWT stored in localStorage — **LOW**
- **Asset:** web front-end + API auth model
- **Evidence:** inline script uses `localStorage.getItem('dgb_session_token')` sent as `Authorization: Bearer <token>`.
- **Impact:** Tokens in `localStorage` are readable by any JavaScript, so an XSS would allow **token theft and user impersonation**. This risk is amplified by the weak CSP (FIND-02).
- **Remediation:** Store the session token in a `HttpOnly; Secure; SameSite=Strict` cookie, or apply strict CSP + short token TTL + refresh rotation as compensating controls.

### FIND-06 — www not canonicalized to apex — **LOW**
- **Evidence:** `www.digiscope.me` serves HTTP 200 identical content (only `<link rel=canonical>` points to apex) instead of a 301 → apex.
- **Impact:** Two trusted origins (both CORS-allowlisted), doubling the trusted-origin surface and enabling potential session/cookie ambiguity.
- **Remediation:** 301-redirect `www` → apex; keep a single canonical origin.

### FIND-07 — Verbose JSON parse error (input echoed) — **INFO**
- **Evidence:** malformed JSON → `{"error":"Unexpected token b in JSON at position 1"}` (body-parser default; echoes input, hints Node/Express).
- **Remediation:** Return a generic `{"error":"Invalid JSON body"}`.

### FIND-08 — Sensitive UI routes disclosed in robots.txt — **INFO**
- **Evidence:** `Disallow: /admin /analytics /my-account /tip-wallet /feedback /digidollar/liquidity /api/` (all client-side SPA routes returning the shell; no server content leaked).
- **Remediation:** robots is not access control — ensure these features enforce server-side authorization; avoid advertising admin paths.

### FIND-09 — Wallet APK without on-page integrity signals — **INFO**
- **Evidence:** `/downloads/digibyte-wallet-v4.0.34.apk` (~89 MB) served with no published SHA-256/PGP signature on the page.
- **Impact:** For a crypto **wallet** binary, lack of verifiable integrity increases tampering/supply-chain risk.
- **Remediation:** Publish signed checksums (SHA-256 + signing-key fingerprint) alongside the download.

### FIND-10 — Legacy SSH SHA-1 HMAC algorithms negotiable — **INFO/LOW**
- **Evidence:** nuclei `ssh-sha1-hmac-algo` on `:22` (OpenSSH 9.6p1).
- **Remediation:** Restrict `MACs` to modern ETM/SHA-2 (e.g. `hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com`).

### FIND-11 — 401 distinguishes missing vs. invalid token / DMARC lacks rua-ruf / no CAA / no security.txt — **INFO (bundle)**
- API returns different messages for "Missing" vs "Invalid" token (minor token-probing feedback → unify to generic 401); DMARC has no `rua`/`ruf` reporting (no spoofing visibility); no CAA record (any CA may issue — recommend pinning to `letsencrypt.org`); no `/.well-known/security.txt` (add a disclosure contact).

---

## 4. Verified Non-Issues (Positive Controls)

- ✅ **No CORS vulnerability** — strict allowlist; only `https://digiscope.me` / `https://www.digiscope.me` reflected with `ACAC:true`; `evil`/`null`/prefix/suffix/`http` origins → 403 (verified on both web and api).
- ✅ **No exposed dotfiles** (`.git`, `.env`, `.DS_Store`, `.htpasswd` → 404); no source maps; no backups/config leaks.
- ✅ **No directory listing** (real dirs → 403, autoindex off).
- ✅ **No API docs/schema exposure** (`/docs`, `/swagger`, `/openapi.json`, `/graphql`, `/metrics` → 404).
- ✅ **Authorization enforced correctly** — protected AI endpoint returns 401 before any side-effect; no unauthenticated access.
- ✅ **Input validation solid** — no XSS reflection on `/c/*` or `/address/*`; path traversal blocked by nginx normalization.
- ✅ **Global rate limiting present** (429 + Retry-After).
- ✅ **Strong TLS** (A-grade, no weak protocols/ciphers) and **strong email posture** (SPF `-all`, DMARC `p=reject`).
- ✅ **No dangerous HTTP methods** (TRACE/PUT/OPTIONS → 405); no version banners.
- ✅ **nuclei:** 0 critical/high/medium, 0 CVE hits, 0 exposed panels, 0 exposures — only informational items.

---

## 5. Overall Posture & Prioritized Remediation Roadmap

**Rating: STRONG / GOOD (B+).** No exploitable web/API vulnerabilities; the single medium risk is an infra-level open resolver. The stack is well-hardened and defensively architected.

**Prioritized roadmap:**

| Priority | Action | Finding | Severity |
|---|---|---|---|
| **1** | Disable/firewall the **open recursive DNS resolver** on :53 (or enable RRL + restrict recursion) | FIND-01 | Medium |
| **2** | Deploy a **strict Content-Security-Policy** on web + API HTML | FIND-02 | Medium |
| **3** | Move session **JWT to HttpOnly/Secure/SameSite cookie** (or enforce CSP + short TTL) | FIND-05 | Low |
| **4** | **Canonicalize www → apex** (301) | FIND-06 | Low |
| **5** | Add **COEP / Permissions-Policy (API) / X-Permitted-Cross-Domain-Policies**; add **HSTS preload** | FIND-03, FIND-04 | Low |
| **6** | Restrict **SSH MACs** to SHA-2/ETM | FIND-10 | Low |
| **7** | Hygiene: publish **APK signed checksums**; generic JSON/401 errors; add **CAA**, **security.txt**, **DMARC rua/ruf** | FIND-07/08/09/11 | Info |

**Assessment complete.** All work was non-destructive identification only, fully within the stated scope and rules of engagement.
