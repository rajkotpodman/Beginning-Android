# Beginning Android Cryptographic & Transport Wire Protocol

This document specifies the exact wire format and cryptographic handshake shared identically between the **Android (Kotlin)** and **Desktop (Go)** clients.

---

## 1. Key Derivation & Pairing

### 1.1 Pair Code
The user is provided with a 6-word mnemonic pair code (e.g., `amber-breeze-cedar-drift-echo-frost`).
- **Normalization**: Trim whitespace, convert to lowercase, hyphen-separated.

### 1.2 Master Seed Derivation (PBKDF2)
- **Algorithm**: `PBKDF2-HMAC-SHA256`
- **Password**: Normalized pair code
- **Salt**: `beginning-android-salt` (UTF-8 bytes)
- **Iterations**: `100,000`
- **Output Length**: 32 bytes (256 bits)
- **Purpose**: Deterministic seed used for initial authentication and room derivation.

### 1.3 Pair Code Hash
- `paircode_hash = SHA-256(normalized_pair_code)` (32 bytes)
- Used as the HKDF salt during the session key derivation.

---

## 2. Asymmetric Key Exchange (X25519)

Each client instance maintains an **X25519** (Curve25519, RFC 7748) keypair:
1. **Private Key**: 32 random bytes, clamped according to RFC 7748:
   - `priv[0] &= 248`
   - `priv[31] &= 127`
   - `priv[31] |= 64`
2. **Public Key**: `X25519(my_priv, base_point_9)` (32 bytes).

### 2.1 Handshake via Blind Relay
When connecting to the relay room WebSocket (`/ws/<pair_code>`), clients exchange their public keys in plaintext envelopes:
```json
{
  "type": "key_exchange",
  "public_key": "<base64_x25519_public_key>",
  "device_name": "Google Pixel 8",
  "platform": "android"
}
```
*Note*: The relay server cannot derive the keys or read any encrypted traffic.

### 2.2 Shared Secret & Session Key
Upon receiving the peer's public key:
1. **Diffie-Hellman Shared Secret**:
   $$\text{shared\_secret} = \text{X25519}(\text{my\_priv}, \text{their\_pub})$$
   (32 bytes)
2. **Session Key Derivation (HKDF-SHA256)**:
   - **IKM**: `shared_secret` (32 bytes)
   - **Salt**: `paircode_hash` (32 bytes)
   - **Info**: `beginning-android-session` (UTF-8 bytes)
   - **Length**: 32 bytes
   $$\text{session\_key} = \text{HKDF-SHA256}(\text{shared\_secret}, \text{salt}=\text{paircode\_hash}, \text{info}=\text{"beginning-android-session"}, L=32)$$

---

## 3. Symmetric Encryption (AES-256-GCM)

All application messages are sealed with AES-256-GCM using `session_key`:
- **Cipher**: `AES-256-GCM` (`AES/GCM/NoPadding`)
- **Nonce / IV**: 12 random bytes (RFC 5116 standard)
- **Tag Size**: 128 bits (16 bytes)
- **Payload Format**: Base64-encoded ciphertext + auth tag.
- **Nonce Format**: Base64-encoded 12-byte nonce.

---

## 4. Replay Protection & Message Counter

To prevent packet injection and replay attacks:
- Each client maintains a strictly monotonic outgoing counter `outgoingSeq` starting at `1`.
- Every encrypted envelope includes a sequential `seq` counter:
```json
{
  "type": "clipboard",
  "seq": 104,
  "payload": "<base64_aes_256_gcm_ciphertext>",
  "nonce": "<base64_12_byte_nonce>"
}
```
- The receiving side verifies that `seq > lastIncomingSeq`. If an envelope arrives with `seq <= lastIncomingSeq`, it is immediately discarded.

---

## 5. File Streaming & 64KB Chunks

Files (general files, documents, or screenshots) are streamed in sequential 64KB chunks:

### 5.1 Initialization (`file_start` or `screenshot`)
```json
{
  "type": "file_start",
  "seq": 105,
  "payload": "<encrypted_json_metadata>",
  "nonce": "<base64_nonce>"
}
```
Decrypted metadata JSON:
```json
{
  "name": "vacation_photo.jpg",
  "size": 2459120,
  "type": "file" // or "screenshot"
}
```

### 5.2 Chunk Streaming (`file_chunk`)
Files are sliced into chunks of maximum **64 KB** (65,536 bytes). Each chunk is encrypted separately:
```json
{
  "type": "file_chunk",
  "seq": 106,
  "chunk_index": 0,
  "payload": "<encrypted_64kb_chunk>",
  "nonce": "<base64_nonce>"
}
```

#### Streaming Backpressure & Flow Control:
To prevent socket buffer overflow, memory spikes, and packet congestion over wireless connections, the sender enforces a **5–10ms backpressure throttling delay** between successive chunk transmissions. This maintains steady throughput and smooth streaming without degrading device responsiveness.

### 5.3 Transfer Completion (`file_end`)
```json
{
  "type": "file_end",
  "seq": 107,
  "payload": "<encrypted_empty_or_checksum>",
  "nonce": "<base64_nonce>"
}
```

---

## 6. Directory Destinations
- **Screenshots**:
  - Windows: `%USERPROFILE%\BeginningAndroid\Screenshots\`
  - Linux: `~/BeginningAndroid/Screenshots/`
  - Android: `Downloads/BeginningAndroid/`
- **Received Files**:
  - Windows: `%USERPROFILE%\BeginningAndroid\Received\`
  - Linux: `~/BeginningAndroid/Received/`
  - Android: `Downloads/BeginningAndroid/`
