# Beginning Android Security Model

## Threat Model & Guarantees
- **E2E Confidentiality**: All clipboard data, screenshots, and files are encrypted client-side using X25519 key exchange + AES-256-GCM.
- **Relay Blindness**: The relay server cannot read or modify payloads without failing cryptographic integrity checks.
- **Man-in-the-Middle (MitM) Prevention**: Keys are verified out-of-band during the initial QR pairing handshake.
