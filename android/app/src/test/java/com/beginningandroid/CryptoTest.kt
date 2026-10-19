package com.beginningandroid

import org.junit.Assert.*
import org.junit.Test

class CryptoTest {

    @Test
    fun testPBKDF2DerivePairSeed() {
        val code1 = "amber-breeze-cedar-drift-echo-frost"
        val code2 = "  AMBER-BREEZE-CEDAR-DRIFT-ECHO-FROST  "
        val code3 = "other-words-here-test-code-alpha"

        val seed1 = Crypto.derivePairSeed(code1)
        val seed2 = Crypto.derivePairSeed(code2)
        val seed3 = Crypto.derivePairSeed(code3)

        assertEquals(32, seed1.size)
        assertArrayEquals(seed1, seed2)
        assertFalse(seed1.contentEquals(seed3))

        // Cross-platform vector verification (matches Go implementation byte-for-byte)
        val expectedSeedHex = "87527f6425b7012ff14a6c8e85c5a7430f3f3acdfe2ed28bd99c7d04ed22f4c1"
        val actualSeedHex = seed1.joinToString("") { "%02x".format(it) }
        assertEquals(expectedSeedHex, actualSeedHex)

        val expectedHashHex = "a867a68f78ea440b4d222da476f3ea5aadeac260d72c0960b12fd2187f31aa01"
        val actualHashHex = Crypto.pairCodeHash(code1).joinToString("") { "%02x".format(it) }
        assertEquals(expectedHashHex, actualHashHex)
    }

    @Test
    fun testX25519KeyExchangeAndHKDF() {
        val pairCode = "amber-breeze-cedar-drift-echo-frost"
        val pairHash = Crypto.pairCodeHash(pairCode)

        // Device A (Android)
        val (privA, pubA) = Crypto.generateX25519KeyPair()

        // Device B (Desktop)
        val (privB, pubB) = Crypto.generateX25519KeyPair()

        // Diffie-Hellman Key Exchange
        val sharedSecretA = Crypto.computeSharedSecret(privA, pubB)
        val sharedSecretB = Crypto.computeSharedSecret(privB, pubA)

        assertArrayEquals(sharedSecretA, sharedSecretB)

        // HKDF-SHA256 session key
        val sessionKeyA = Crypto.deriveSessionKey(sharedSecretA, pairHash)
        val sessionKeyB = Crypto.deriveSessionKey(sharedSecretB, pairHash)

        assertEquals(32, sessionKeyA.size)
        assertArrayEquals(sessionKeyA, sessionKeyB)

        // AES-256-GCM encryption & decryption
        val message = "Hello from Android test suite! 🚀"
        val (payload, nonce) = Crypto.encryptString(sessionKeyA, message)
        val decrypted = Crypto.decryptString(sessionKeyB, payload, nonce)

        assertEquals(message, decrypted)
    }
}
