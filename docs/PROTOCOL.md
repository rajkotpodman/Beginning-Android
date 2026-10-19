# Beginning Android Transport & Wire Protocol

## Wire Format
All messages exchanged over the WebSocket connection adhere to the following encrypted envelope:

```json
{
  "version": 1,
  "session_id": "<hex_session_id>",
  "nonce": "<base64_nonce>",
  "sender_id": "<hex_sender_public_key_fingerprint>",
  "ciphertext": "<base64_aes_256_gcm_payload>"
}
```

## Payload Types (Encrypted)
- `CLIPBOARD_SYNC`: Text clipboard content with origin timestamp.
- `SCREENSHOT_TRANSFER`: Metadata + binary image chunk stream.
- `FILE_TRANSFER_INIT`: File name, total size, hash.
- `FILE_TRANSFER_CHUNK`: Sequential file data chunk.
- `PING / PONG`: Liveness check.
