# Beginning Android Architecture

## High-Level Design
The system consists of three primary components:
1. **Android Client (Kotlin)**: Background services for clipboard monitoring and screenshot observation, paired with a lightweight UI.
2. **Desktop Client (Go)**: Cross-platform headless/systray client for Windows and Linux with direct clipboard APIs and native context menu integration.
3. **Relay Server (Go)**: Blind, stateless WebSocket forwarder enabling NAT traversal and seamless device connectivity without third-party cloud storage.
