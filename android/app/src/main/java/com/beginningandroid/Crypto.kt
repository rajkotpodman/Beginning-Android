package com.beginningandroid

import java.math.BigInteger
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.Mac
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.PBEKeySpec
import javax.crypto.spec.SecretKeySpec

object Crypto {
    private const val AES_GCM_NO_PADDING = "AES/GCM/NoPadding"
    private const val GCM_TAG_LENGTH_BITS = 128
    private const val NONCE_LENGTH_BYTES = 12
    private const val PBKDF2_ITERATIONS = 100000
    private const val KEY_LENGTH_BITS = 256
    private const val PBKDF2_SALT = "beginning-android-salt"
    private const val HKDF_INFO = "beginning-android-session"

    private val secureRandom = SecureRandom()

    // Curve25519 constants (RFC 7748)
    private val P: BigInteger = BigInteger.valueOf(2).pow(255).subtract(BigInteger.valueOf(19))
    private val A24: BigInteger = BigInteger.valueOf(121665)
    private val BASE_POINT = ByteArray(32).apply { this[0] = 9 }

    fun normalizePairCode(pairCode: String): String {
        return pairCode.trim().lowercase()
    }

    /**
     * Pair code (6 words) -> PBKDF2-HMAC-SHA256 (100k iters) -> 32-byte seed
     */
    fun derivePairSeed(pairCode: String): ByteArray {
        val cleanCode = normalizePairCode(pairCode)
        val spec = PBEKeySpec(
            cleanCode.toCharArray(),
            PBKDF2_SALT.toByteArray(StandardCharsets.UTF_8),
            PBKDF2_ITERATIONS,
            KEY_LENGTH_BITS
        )
        val factory = SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256")
        return factory.generateSecret(spec).encoded
    }

    /**
     * Computes SHA-256 hash of the normalized pair code (used as HKDF salt)
     */
    fun pairCodeHash(pairCode: String): ByteArray {
        val digest = MessageDigest.getInstance("SHA-256")
        return digest.digest(normalizePairCode(pairCode).toByteArray(StandardCharsets.UTF_8))
    }

    /**
     * Generates a random X25519 private key and derives its public key (RFC 7748)
     */
    fun generateX25519KeyPair(): Pair<ByteArray, ByteArray> {
        val priv = ByteArray(32)
        secureRandom.nextBytes(priv)
        val pub = scalarMult(priv, BASE_POINT)
        return Pair(priv, pub)
    }

    /**
     * Shared secret = X25519(my_priv, their_pub)
     */
    fun computeSharedSecret(myPriv: ByteArray, theirPub: ByteArray): ByteArray {
        return scalarMult(myPriv, theirPub)
    }

    /**
     * Session key = HKDF-SHA256(shared_secret, salt=paircode_hash, info="beginning-android-session")
     */
    fun deriveSessionKey(sharedSecret: ByteArray, pairCodeHash: ByteArray): ByteArray {
        val mac = Mac.getInstance("HmacSHA256")
        // HKDF-Extract: PRK = HMAC-Hash(salt, IKM)
        mac.init(SecretKeySpec(pairCodeHash, "HmacSHA256"))
        val prk = mac.doFinal(sharedSecret)

        // HKDF-Expand: OKM = HMAC-Hash(PRK, info || 0x01)
        mac.init(SecretKeySpec(prk, "HmacSHA256"))
        mac.update(HKDF_INFO.toByteArray(StandardCharsets.UTF_8))
        mac.update(1.toByte())
        val okm = mac.doFinal()
        return okm.copyOf(32)
    }

    /**
     * Encrypts plaintext using AES-256-GCM with a random 12-byte nonce
     */
    fun encrypt(keyBytes: ByteArray, plaintextBytes: ByteArray): Pair<String, String> {
        val nonce = ByteArray(NONCE_LENGTH_BYTES)
        secureRandom.nextBytes(nonce)

        val cipher = Cipher.getInstance(AES_GCM_NO_PADDING)
        val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce)
        cipher.init(Cipher.ENCRYPT_MODE, SecretKeySpec(keyBytes, "AES"), spec)

        val ciphertext = cipher.doFinal(plaintextBytes)
        val payloadBase64 = Base64.getEncoder().encodeToString(ciphertext)
        val nonceBase64 = Base64.getEncoder().encodeToString(nonce)

        return Pair(payloadBase64, nonceBase64)
    }

    fun encryptString(keyBytes: ByteArray, text: String): Pair<String, String> {
        return encrypt(keyBytes, text.toByteArray(StandardCharsets.UTF_8))
    }

    /**
     * Decrypts ciphertext using AES-256-GCM
     */
    fun decrypt(keyBytes: ByteArray, payloadBase64: String, nonceBase64: String): ByteArray {
        val ciphertext = Base64.getDecoder().decode(payloadBase64)
        val nonce = Base64.getDecoder().decode(nonceBase64)

        val cipher = Cipher.getInstance(AES_GCM_NO_PADDING)
        val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce)
        cipher.init(Cipher.DECRYPT_MODE, SecretKeySpec(keyBytes, "AES"), spec)

        return cipher.doFinal(ciphertext)
    }

    fun decryptString(keyBytes: ByteArray, payloadBase64: String, nonceBase64: String): String {
        val decryptedBytes = decrypt(keyBytes, payloadBase64, nonceBase64)
        return String(decryptedBytes, StandardCharsets.UTF_8)
    }

    // --- RFC 7748 X25519 Implementation ---
    private fun scalarMult(scalarBytes: ByteArray, uBytes: ByteArray): ByteArray {
        val k = scalarBytes.copyOf()
        k[0] = (k[0].toInt() and 248).toByte()
        k[31] = (k[31].toInt() and 127 or 64).toByte()
        val kVal = decodeLittleEndian(k)

        val u = uBytes.copyOf()
        u[31] = (u[31].toInt() and 127).toByte()
        val x1 = decodeLittleEndian(u).mod(P)

        var x2 = BigInteger.ONE
        var z2 = BigInteger.ZERO
        var x3 = x1
        var z3 = BigInteger.ONE
        var swap = 0

        for (t in 254 downTo 0) {
            val kt = if (kVal.testBit(t)) 1 else 0
            swap = swap xor kt
            if (swap != 0) {
                val tx = x2; x2 = x3; x3 = tx
                val tz = z2; z2 = z3; z3 = tz
            }
            swap = kt

            val a = (x2 + z2).mod(P)
            val aa = (a * a).mod(P)
            val b = (x2 - z2).mod(P)
            val bb = (b * b).mod(P)
            val e = (aa - bb).mod(P)
            val c = (x3 + z3).mod(P)
            val d = (x3 - z3).mod(P)
            val da = (d * a).mod(P)
            val cb = (c * b).mod(P)

            x3 = ((da + cb).pow(2)).mod(P)
            z3 = (x1 * (da - cb).pow(2)).mod(P)
            x2 = (aa * bb).mod(P)
            z2 = (e * (aa + A24 * e)).mod(P)
        }

        if (swap != 0) {
            x2 = x3
            z2 = z3
        }

        val result = (x2 * z2.modPow(P.subtract(BigInteger.valueOf(2)), P)).mod(P)
        return encodeLittleEndian(result)
    }

    private fun decodeLittleEndian(bytes: ByteArray): BigInteger {
        val reversed = bytes.reversedArray()
        return BigInteger(1, reversed)
    }

    private fun encodeLittleEndian(value: BigInteger): ByteArray {
        val raw = value.toByteArray()
        val out = ByteArray(32)
        val copyLen = minOf(raw.size, 32)
        for (i in 0 until copyLen) {
            out[i] = raw[raw.size - 1 - i]
        }
        return out
    }
}
