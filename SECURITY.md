# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

Please report security vulnerabilities privately to the maintainers.

**Do not open a public issue.**

We aim to acknowledge reports within 48 hours and provide a fix or mitigation
within 90 days, depending on complexity.

## Framework Security

Aku is built on Go's `net/http` standard library and follows these principles:

- **No default unsafe behavior** — Secure defaults for timeouts, body size limits,
  and CORS.
- **Middleware opt-in** — Security headers, rate limiting, and CORS are available
  as middleware but must be explicitly configured.
- **Problem Details (RFC 9457)** — Error responses follow the Problem Details
  standard, avoiding information leakage through stack traces.
- **Dependency hygiene** — Dependencies are kept minimal and updated regularly.
  Vulnerability scanning is part of CI.

## What Aku Does NOT Handle

These are outside the framework's scope and must be handled by your application
or middleware:

- CSRF protection
- Session management
- Authentication logic (Aku provides typed extractors for Bearer tokens and API
  keys, but not session stores or OAuth flows)
- Rate limiting by user/IP (the built-in `Limit` middleware is a simple global
  limiter)
