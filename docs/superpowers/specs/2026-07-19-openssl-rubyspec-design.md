# OpenSSL RubySpec Compatibility Design

## Goal

Implement the OpenSSL behavior exercised by `vendor/ruby/spec/library/openssl` with native, deterministic RGo objects and Go cryptography. Success is the complete OpenSSL spec directory reporting zero failures under the project's single-core test limits.

## Architecture

Install `OpenSSL` from the core library loader. Keep the implementation in a dedicated `pkg/core/openssl.go` file and reuse the existing Digest algorithm definitions rather than duplicating hash behavior.

The module contains:

- `OpenSSL::Random`: cryptographically secure binary strings via `crypto/rand`.
- `OpenSSL::Digest`: stateful SHA1/SHA256/SHA384/SHA512 objects, including generic and named constructors, update/reset, digest forms, lengths, and names.
- `OpenSSL::HMAC`: raw and hexadecimal HMAC using the selected Digest object.
- `OpenSSL::KDF`: PBKDF2-HMAC and Scrypt with Ruby-compatible keyword coercion, validation, errors, and binary output.
- `OpenSSL.secure_compare` and `.fixed_length_secure_compare`: constant-time byte comparison while preserving Ruby coercion and identity/equality rules.
- `OpenSSL::X509`, `ASN1`, and `PKey`: the certificate/name/store state needed by the covered verification scenarios. Certificate validity is based on validity dates and the store's trusted certificates; signing records issuer/key relationships without pretending to provide general-purpose DER or TLS support.

## Data and Error Semantics

Digest and certificate state live in typed Go structs stored on Ruby objects. Raw cryptographic output uses ASCII-8BIT; hexadecimal and algorithm names use ASCII-compatible strings. All String and Integer inputs follow existing RGo `to_str` and `to_int` coercion helpers.

Unknown algorithms raise `OpenSSL::Digest::DigestError`; invalid KDF parameters raise `OpenSSL::KDF::KDFError` or `ArgumentError` according to RubySpec. Missing keywords retain the VM's normal Ruby keyword errors. Random lengths reject negatives before allocation.

## Delivery Order

1. Module/constants, secure comparison, and Random.
2. Digest and HMAC, reusing the verified Digest engine.
3. PBKDF2-HMAC and Scrypt.
4. X509 Name parsing and certificate/store verification state.
5. Full OpenSSL directory refresh and focused Go regression suite.

Each step starts with a failing focused regression, then runs only the affected RubySpec files. The final gate runs all 19 OpenSSL files sequentially with `GOMAXPROCS=1`, `GOFLAGS=-p=1`, `nice`, and per-file timeouts. No Git commit is performed.

## Scope Boundary

This stage implements the repository's authoritative OpenSSL RubySpec surface. APIs not present in that suite, such as live TLS sockets or arbitrary PEM/DER interoperability, remain separate work and must not be claimed as verified by this gate.
