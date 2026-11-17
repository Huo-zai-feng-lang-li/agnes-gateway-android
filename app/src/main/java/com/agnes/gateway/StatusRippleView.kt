package com.agnes.gateway

import android.animation.ValueAnimator
import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.util.AttributeSet
import android.view.View
import android.view.animation.LinearInterpolator

/**
 * 顶部状态点：中心实心圆 + 两圈向外扩散并淡出的水波纹。
 * 只负责绘制与动画，运行/停止由外部控制。
 */
class StatusRippleView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
    defStyleAttr: Int = 0
) : View(context, attrs, defStyleAttr) {

    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val density = resources.displayMetrics.density
    private val dotRadius = 4.5f * density
    private val maxRadius = 18f * density
    private var rippleAnimator: ValueAnimator? = null
    private var progress = 0f
    private var rippleColor = 0xFF00C853.toInt()
    private var running = false

    /** 修改圆点与波纹颜色，不改动当前动画进度。 */
    fun setRippleColor(color: Int) {
        rippleColor = color
        invalidate()
    }

    /** 启动水波纹循环动画；重复调用不会重启动画。 */
    fun startRipple() {
        if (running) return
        running = true
        rippleAnimator = ValueAnimator.ofFloat(0f, 1f).apply {
            duration = 1400
            repeatCount = ValueAnimator.INFINITE
            interpolator = LinearInterpolator()
            addUpdateListener {
                progress = it.animatedValue as Float
                invalidate()
            }
            start()
        }
    }

    /** 停止波纹并回到静态圆点。 */
    fun stopRipple() {
        running = false
        rippleAnimator?.cancel()
        rippleAnimator = null
        progress = 0f
        invalidate()
    }

    override fun onDetachedFromWindow() {
        stopRipple()
        super.onDetachedFromWindow()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        val cx = width / 2f
        val cy = height / 2f

        if (running) {
            drawRing(canvas, cx, cy, progress)
            drawRing(canvas, cx, cy, (progress + 0.5f) % 1f)
        }

        paint.style = Paint.Style.FILL
        paint.color = rippleColor
        canvas.drawCircle(cx, cy, dotRadius, paint)
    }

    private fun drawRing(canvas: Canvas, cx: Float, cy: Float, phase: Float) {
        val radius = dotRadius + (maxRadius - dotRadius) * phase
        val alpha = ((1f - phase) * 140f).toInt().coerceIn(0, 255)
        paint.style = Paint.Style.STROKE
        paint.strokeWidth = 1.5f * density
        paint.color = Color.argb(
            alpha,
            Color.red(rippleColor),
            Color.green(rippleColor),
            Color.blue(rippleColor)
        )
        canvas.drawCircle(cx, cy, radius, paint)
    }
}
