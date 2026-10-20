package com.beginningandroid

import android.content.Context
import android.graphics.*
import android.os.SystemClock
import android.util.AttributeSet
import android.view.View
import kotlin.math.sin

/**
 * GeminiWaveVisualizerView
 * 
 * Hardware-accelerated 60fps dynamic audio wave graph visualizer
 * inspired by Google Gemini Live Voice chat interface.
 * Reacts dynamically to microphone RMS decibels.
 */
class GeminiWaveVisualizerView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = 0
) : View(context, attrs, defStyleAttr) {

    private val wavePaint1 = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 5f
        strokeCap = Paint.Cap.ROUND
    }
    private val wavePaint2 = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 4f
        strokeCap = Paint.Cap.ROUND
    }
    private val wavePaint3 = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 3.5f
        strokeCap = Paint.Cap.ROUND
    }
    private val wavePaint4 = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 3f
        strokeCap = Paint.Cap.ROUND
    }

    private val path1 = Path()
    private val path2 = Path()
    private val path3 = Path()
    private val path4 = Path()

    private var targetAmplitude = 0.08f
    private var currentAmplitude = 0.08f
    private var phase = 0f
    private var lastFrameTime = SystemClock.uptimeMillis()
    private var isActive = false

    init {
        // Set up software/hardware layer if needed
        setLayerType(LAYER_TYPE_HARDWARE, null)
    }

    override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) {
        super.onSizeChanged(w, h, oldw, oldh)
        if (w > 0 && h > 0) {
            val widthF = w.toFloat()
            // Gemini iridescent gradient: Cyan -> Purple -> Blue -> Teal
            val shader1 = LinearGradient(
                0f, 0f, widthF, 0f,
                intArrayOf(0xFF38BDF8.toInt(), 0xFF818CF8.toInt(), 0xFFC084FC.toInt(), 0xFF0D9488.toInt()),
                floatArrayOf(0f, 0.35f, 0.7f, 1f),
                Shader.TileMode.CLAMP
            )
            val shader2 = LinearGradient(
                0f, 0f, widthF, 0f,
                intArrayOf(0xFFC084FC.toInt(), 0xFF38BDF8.toInt(), 0xFF10B981.toInt(), 0xFF6366F1.toInt()),
                floatArrayOf(0f, 0.3f, 0.65f, 1f),
                Shader.TileMode.CLAMP
            )
            wavePaint1.shader = shader1
            wavePaint2.shader = shader2
            wavePaint3.shader = shader1
            wavePaint4.shader = shader2
        }
    }

    fun setActive(active: Boolean) {
        isActive = active
        if (!active) {
            targetAmplitude = 0.04f
        }
        invalidate()
    }

    fun setRms(rmsdB: Float) {
        if (!isActive) return
        // rmsdB typically ranges from -2.0 to 12.0 in Android SpeechRecognizer
        val normalized = ((rmsdB + 2f) / 14f).coerceIn(0.1f, 1.0f)
        targetAmplitude = normalized
        invalidate()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)

        val w = width.toFloat()
        val h = height.toFloat()
        if (w <= 0f || h <= 0f) return

        val centerY = h / 2f
        val now = SystemClock.uptimeMillis()
        val dt = (now - lastFrameTime).coerceAtMost(50L) / 1000f
        lastFrameTime = now

        // Smooth amplitude interpolation
        val lerpFactor = if (targetAmplitude > currentAmplitude) 14f * dt else 8f * dt
        currentAmplitude += (targetAmplitude - currentAmplitude) * lerpFactor.coerceIn(0.01f, 0.95f)

        // Advance wave phase
        val speed = if (isActive) 5.5f else 1.8f
        phase += speed * dt

        val maxAmp = (h * 0.42f) * currentAmplitude

        // Draw 4 distinct harmonic sinusoidal paths
        drawHarmonicWave(canvas, path1, wavePaint1, centerY, maxAmp, w, phase, 1.2f, 0f)
        drawHarmonicWave(canvas, path2, wavePaint2, centerY, maxAmp * 0.82f, w, phase * 0.85f, 1.6f, 1.2f)
        drawHarmonicWave(canvas, path3, wavePaint3, centerY, maxAmp * 0.65f, w, phase * 1.15f, 2.1f, 2.4f)
        drawHarmonicWave(canvas, path4, wavePaint4, centerY, maxAmp * 0.5f, w, phase * 0.7f, 0.9f, 3.8f)

        // Keep animating smoothly at 60 FPS while active or settling
        if (isActive || currentAmplitude > 0.05f) {
            postInvalidateOnAnimation()
        }
    }

    private fun drawHarmonicWave(
        canvas: Canvas,
        path: Path,
        paint: Paint,
        centerY: Float,
        amplitude: Float,
        width: Float,
        basePhase: Float,
        frequency: Float,
        phaseOffset: Float
    ) {
        path.reset()
        val step = 4f
        var x = 0f
        var first = true

        while (x <= width) {
            // Window envelope: fade out at left and right edges (Tukey/Hann window)
            val normalizedX = x / width
            val envelope = sin(normalizedX * Math.PI).toFloat().coerceIn(0f, 1f)

            // Sine modulation
            val angle = (normalizedX * Math.PI * 2.0 * frequency + basePhase + phaseOffset).toFloat()
            val y = centerY + sin(angle) * amplitude * envelope

            if (first) {
                path.moveTo(x, y)
                first = false
            } else {
                path.lineTo(x, y)
            }
            x += step
        }

        canvas.drawPath(path, paint)
    }
}
