# OpenSSL RubySpec Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every spec under `vendor/ruby/spec/library/openssl` pass with native RGo behavior.

**Architecture:** Add a focused `pkg/core/openssl.go` installer and typed state objects. Reuse the existing Digest algorithm table, use Go's cryptographic primitives, implement PBKDF2/Scrypt locally to avoid a network dependency, and model only the X509 state exercised by the repository specs.

**Tech Stack:** Go 1.24, `crypto/rand`, `crypto/hmac`, `crypto/subtle`, existing RGo object model and RubySpec runner.

---

### Task 1: Loader, Random, and secure comparison

**Files:**
- Create: `pkg/core/openssl.go`
- Modify: `pkg/core/init.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestOpenSSLRandomAndSecureCompare` asserting random byte length/ASCII-8BIT encoding, distinct values, negative-length `ArgumentError`, equal/different comparisons, `to_str`, and unequal-length behavior.
- [ ] Run:

```sh
env GOMAXPROCS=1 GOFLAGS=-p=1 nice -n 10 timeout 60s go test ./pkg/vm -run TestOpenSSLRandomAndSecureCompare -count=1
```

Expected: FAIL because `OpenSSL` is undefined.

- [ ] Create `installOpenSSLModule` and register `require "openssl"` beside the Digest loader. Define version constants, `OpenSSL::Random.random_bytes`, `pseudo_bytes`, `OpenSSL.fixed_length_secure_compare`, and `secure_compare`. Core comparison logic is:

```go
func opensslFixedCompare(a, b []byte) (bool, error) {
	if len(a) != len(b) { return false, errors.New("inputs must be of equal length") }
	return subtle.ConstantTimeCompare(a, b) == 1, nil
}
```

Return random data with `stringWithEncoding(string(buf), "ASCII-8BIT")`; coerce both comparison operands through `evalCoerceToString` and preserve `secure_compare`'s original-object equality check.

- [ ] Run the Go test plus both comparison specs and both Random specs; expect zero failures.

### Task 2: OpenSSL Digest and HMAC

**Files:**
- Modify: `pkg/core/openssl.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestOpenSSLDigestAndHMAC` covering generic/named constructors, initial data, reset, name/lengths, raw/hex/base64 output, update aliases, and the SHA1 HMAC fixture.
- [ ] Run the focused test and confirm failure from missing Digest/HMAC constants.
- [ ] Define `opensslDigestState { spec *digestAlgorithmSpec; data []byte }`. Install generic `OpenSSL::Digest.new(nameOrDigest, initial=nil)`, named SHA classes, class `digest/hexdigest/base64digest`, and instance methods `update`, `<<`, `reset`, `digest`, `hexdigest`, `name`, `digest_length`, and `block_length`. Resolve names case-insensitively only across SHA1/SHA256/SHA384/SHA512 and raise `OpenSSL::Digest::DigestError` otherwise.
- [ ] Implement HMAC through:

```go
mac := hmac.New(state.spec.newHash, []byte(key))
_, _ = mac.Write([]byte(data))
sum := mac.Sum(nil)
```

Return raw HMAC as ASCII-8BIT and hexadecimal HMAC as US-ASCII.
- [ ] Run all eight Digest specs and both HMAC specs; expect zero failures.

### Task 3: PBKDF2-HMAC and Scrypt

**Files:**
- Modify: `pkg/core/openssl.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestOpenSSLKDFVectorsAndValidation` using the exact PBKDF2 and Scrypt vectors from the RubySpec files, including embedded NUL, arbitrary output length, coercion, missing keywords, and invalid cost values.
- [ ] Run the test and confirm missing `OpenSSL::KDF` failures.
- [ ] Implement PBKDF2 block expansion locally:

```go
for block := uint32(1); len(out) < length; block++ {
	u := hmacSum(password, append(salt, byte(block>>24), byte(block>>16), byte(block>>8), byte(block)))
	t := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ { u = hmacSum(password, u); xorBytes(t, u) }
	out = append(out, t...)
}
```

Validate and coerce `salt`, `iterations`, `length`, and `hash` from the VM's keyword hash.
- [ ] Implement RFC 7914 Scrypt with PBKDF2-HMAC-SHA256, BlockMix, Integerify, ROMix, and Salsa20/8. Require power-of-two `N > 1`, positive `r/p`, nonnegative length, and overflow-safe allocation checks; return `KDFError` for cost failures.
- [ ] Run `pbkdf2_hmac_spec.rb` and `scrypt_spec.rb`; expect zero failures under the per-file timeout.

### Task 4: X509 Name and verification model

**Files:**
- Modify: `pkg/core/openssl.go`
- Test: `pkg/vm/executor_test.go`

- [ ] Add `TestOpenSSLX509NameAndStoreValidity` covering slash/comma names, ASN1 types, invalid DNs, valid self-signed certificate, expired leaf, and expired trusted root.
- [ ] Run the focused test and confirm missing X509/PKey failures.
- [ ] Define typed states for Name entries, RSA key identity, Certificate fields, ExtensionFactory, and Store. Add ordinary attribute readers/writers required by the specs. `Name.parse` accepts `/K=V` and comma-separated `K=V`, restricts keys to recognized DN keys, maps `DC` to IA5STRING and `CN` to UTF8STRING, and normalizes `to_s` to slash form.
- [ ] `Certificate#sign` records signer identity; `Store#verify` sets `[error,error_string]` to `[0,"ok"]` only when the leaf and applicable trusted issuer are valid at `Time.now`. Any expired leaf or trusted issuer returns false.
- [ ] Run both X509 files; expect zero failures.

### Task 5: Full OpenSSL gate and regressions

**Files:**
- Modify: `TODO.md`

- [ ] Format changed Go files and run all focused OpenSSL Go tests serially.
- [ ] Build `rgo` with one compiler worker.
- [ ] Run:

```sh
env BUILD_BINARY=0 RGO_SPEC_TIMEOUT=20 GOMAXPROCS=1 GOFLAGS=-p=1 nice -n 10 timeout 180s scripts/spec_status.sh vendor/ruby/spec/library/openssl /tmp/rgo-openssl-final.csv
```

Expected: every OpenSSL file is `pass` or a version-guarded `zero_examples`, with zero failures and no runtime errors/timeouts.

- [ ] Run the existing Digest and SecureRandom focused Go tests to prove shared cryptographic behavior did not regress.
- [ ] Record exact file/example/failure totals in `TODO.md`. Do not stage or commit any files.
