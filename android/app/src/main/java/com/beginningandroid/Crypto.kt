package com.beginningandroid

import android.util.Base64
import java.nio.charset.StandardCharsets
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

object Crypto {
    private const val AES_GCM_NO_PADDING = "AES/GCM/NoPadding"
    private const val GCM_TAG_LENGTH_BITS = 128
    private const val NONCE_LENGTH_BYTES = 12
    private val secureRandom = SecureRandom()

    fun deriveKey(pairCode: String): SecretKeySpec {
        val digest = MessageDigest.getInstance("SHA-256")
        val keyBytes = digest.digest(pairCode.trim().lowercase().toByteArray(StandardCharsets.UTF_8))
        return SecretKeySpec(keyBytes, "AES")
    }

    fun encrypt(key: SecretKeySpec, plaintextBytes: ByteArray): Pair<String, String> {
        val nonce = ByteArray(NONCE_LENGTH_BYTES)
        secureRandom.nextBytes(nonce)

        val cipher = Cipher.getInstance(AES_GCM_NO_PADDING)
        val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce)
        cipher.init(Cipher.ENCRYPT_MODE, key, spec)

        val ciphertext = cipher.doFinal(plaintextBytes)
        val payloadBase64 = Base64.encodeToString(ciphertext, Base64.NO_WRAP)
        val nonceBase64 = Base64.encodeToString(nonce, Base64.NO_WRAP)

        return Pair(payloadBase64, nonceBase64)
    }

    fun encryptString(key: SecretKeySpec, text: String): Pair<String, String> {
        return encrypt(key, text.toByteArray(StandardCharsets.UTF_8))
    }

    fun decrypt(key: SecretKeySpec, payloadBase64: String, nonceBase64: String): ByteArray {
        val ciphertext = Base64.decode(payloadBase64, Base64.NO_WRAP)
        val nonce = Base64.decode(nonceBase64, Base64.NO_WRAP)

        val cipher = Cipher.getInstance(AES_GCM_NO_PADDING)
        val spec = GCMParameterSpec(GCM_TAG_LENGTH_BITS, nonce)
        cipher.init(Cipher.DECRYPT_MODE, key, spec)

        return cipher.doFinal(ciphertext)
    }

    fun decryptString(key: SecretKeySpec, payloadBase64: String, nonceBase64: String): String {
        val decryptedBytes = decrypt(key, payloadBase64, nonceBase64)
        return String(decryptedBytes, StandardCharsets.UTF_8)
    }
}
