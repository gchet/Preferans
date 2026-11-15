package com.preferans.game

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.IBinder
import android.os.PowerManager

/**
 * Служба переднего плана на время партии: пока игрок за столом, процесс не усыпляют при блокировке экрана или
 * сворачивании, а Wi-Fi и процессор не переходят в экономный режим. Без неё на телефоне ведущего связь со всеми
 * участниками рвалась разом (тайм-аут «нет ответа канала» у всех игроков одновременно).
 */
class KeepAliveService : Service() {
    private var wifiLock: WifiManager.WifiLock? = null
    private var wakeLock: PowerManager.WakeLock? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val channelId = "keepalive"
        val manager = getSystemService(NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(
                NotificationChannel(channelId, intent?.getStringExtra("channel") ?: "Preferans", NotificationManager.IMPORTANCE_LOW)
            )
        }
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_REORDER_TO_FRONT),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        )
        val builder = if (Build.VERSION.SDK_INT >= 26) Notification.Builder(this, channelId) else @Suppress("DEPRECATION") Notification.Builder(this)
        val notification = builder
            .setSmallIcon(applicationInfo.icon)
            .setContentTitle(intent?.getStringExtra("title") ?: "Preferans")
            .setContentText(intent?.getStringExtra("text") ?: "")
            .setContentIntent(open)
            .setOngoing(true)
            .build()
        if (Build.VERSION.SDK_INT >= 29) startForeground(1, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        else startForeground(1, notification)
        acquireLocks()
        return START_NOT_STICKY
    }

    @Suppress("DEPRECATION")
    private fun acquireLocks() {
        try {
            if (wifiLock?.isHeld != true) {
                val wifi = applicationContext.getSystemService(Context.WIFI_SERVICE) as WifiManager
                val mode = if (Build.VERSION.SDK_INT >= 29) WifiManager.WIFI_MODE_FULL_LOW_LATENCY else WifiManager.WIFI_MODE_FULL_HIGH_PERF
                wifiLock = wifi.createWifiLock(mode, "preferans:wifi").apply { setReferenceCounted(false); acquire() }
            }
        } catch (_: Exception) {}
        try {
            if (wakeLock?.isHeld != true) {
                val power = getSystemService(POWER_SERVICE) as PowerManager
                // Не дольше 6 часов: партия не длится дольше, а забытая блокировка не посадит батарею.
                wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "preferans:game").apply {
                    setReferenceCounted(false)
                    acquire(6 * 60 * 60 * 1000L)
                }
            }
        } catch (_: Exception) {}
    }

    private fun releaseLocks() {
        try { wifiLock?.takeIf { it.isHeld }?.release() } catch (_: Exception) {}
        try { wakeLock?.takeIf { it.isHeld }?.release() } catch (_: Exception) {}
        wifiLock = null
        wakeLock = null
    }

    // Android 15 ограничивает службы типа dataSync шестью часами в сутки: останавливаемся, а не падаем.
    override fun onTimeout(startId: Int, fgsType: Int) {
        stopSelf()
    }

    override fun onDestroy() {
        releaseLocks()
        super.onDestroy()
    }
}
