package com.preferans.game

import android.app.Activity
import android.app.KeyguardManager
import android.Manifest
import android.content.BroadcastReceiver
import android.content.Context
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.webkit.PermissionRequest
import android.content.Intent
import android.content.ContentValues
import android.os.Environment
import android.provider.MediaStore
import android.media.MediaScannerConnection
import android.content.pm.ApplicationInfo
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.Uri
import android.os.Bundle
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.media.AudioDeviceCallback
import android.media.AudioDeviceInfo
import android.media.AudioManager
import android.provider.Settings
import android.webkit.ValueCallback
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.TextView
import mobile.Mobile
import java.io.File
import java.net.HttpURLConnection
import java.net.URL
import org.json.JSONObject

class MainActivity : Activity() {
    // The exact same catalog is embedded by Go and imported by WebUI.
    @Volatile private var language = "uk"
    private val messages by lazy {
        assets.open("locales/ru.json").bufferedReader(Charsets.UTF_8).use { JSONObject(it.readText()) }
    }
    private val catalogs by lazy {
        mapOf("ru" to messages) + listOf("uk", "en").associateWith { code ->
            assets.open("locales/$code.json").bufferedReader(Charsets.UTF_8).use { JSONObject(it.readText()) }
        }
    }
    private fun text(key: String, vararg values: Any?): String {
        val template = catalogs[language]?.optString(key).orEmpty().ifEmpty { messages.optString(key, key) }
        return Regex("\\{p(\\d+)\\}").replace(template) { match ->
            val index = match.groupValues[1].toInt()
            if (index < values.size) values[index].toString() else match.value
        }
    }
    private lateinit var web: WebView
    private var files: ValueCallback<Array<Uri>>? = null
    private var pendingCode: String? = null
    private var microphoneRequest: PermissionRequest? = null
    private var exiting = false
    private var exportingLog = false
    private var keepAliveOn = false
    private val screenOffReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == Intent.ACTION_SCREEN_OFF && ::web.isInitialized)
                web.evaluateJavascript("window.dispatchEvent(new Event('preferans-voice-lock'))", null)
        }
    }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        language = getSharedPreferences("interface", MODE_PRIVATE).getString("language", "uk") ?: "uk"
        try {
            val zone = java.util.TimeZone.getDefault()
            Mobile.setTimeZone(zone.id, (zone.getOffset(System.currentTimeMillis()) / 1000).toLong())
            Mobile.setSecretCipher(ApiKeyCipher())
            val url = Mobile.start(filesDir.absolutePath)
            watchNetwork()
            val origin = Uri.parse(url)
            web = WebView(this)
            WebView.setWebContentsDebuggingEnabled((applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE) != 0)
            web.settings.javaScriptEnabled = true
            web.settings.domStorageEnabled = true
            web.settings.mediaPlaybackRequiresUserGesture = false
            web.settings.allowFileAccess = false
            web.settings.allowContentAccess = true
            web.addJavascriptInterface(object {
                @JavascriptInterface fun setLanguage(value: String) {
                    if (value !in setOf("ru", "uk", "en") || value == language) return
                    language = value
                    getSharedPreferences("interface", MODE_PRIVATE).edit().putString("language", value).apply()
                }
                @JavascriptInterface fun exitApp() {
                    runOnUiThread {
                        if (!exiting) {
                            exiting = true
                            finishAndRemoveTask()
                        }
                    }
                }
                @JavascriptInterface fun saveCode(code: String) {
                    if (code.length > 131072 || !code.startsWith("PREF1.")) return
                    runOnUiThread {
                        pendingCode = code
                        startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                            addCategory(Intent.CATEGORY_OPENABLE)
                            type = "text/plain"
                            putExtra(Intent.EXTRA_TITLE, "preferans-code.txt")
                        }, 2)
                    }
                }
                @JavascriptInterface fun saveLog(log: String) {
                    if (log.length > 1048576) return
                    runOnUiThread {
                        pendingCode = log
                        startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                            addCategory(Intent.CATEGORY_OPENABLE)
                            type = "text/plain"
                            putExtra(Intent.EXTRA_TITLE, "preferans.log")
                        }, 2)
                    }
                }
                @JavascriptInterface fun saveFullLogToDownloads() {
                    runOnUiThread { exportLogToDownloads() }
                }
                // Пока игрок за столом, служба переднего плана не даёт ОС усыпить связь (блокировка экрана, сворачивание).
                @JavascriptInterface fun setKeepAlive(on: Boolean) {
                    runOnUiThread { if (on) startKeepAlive() else stopKeepAlive() }
                }
                // Фиксированный звуковой режим на время голосовой связи: громкость собеседников не прыгает при включении и выключении микрофона.
                @JavascriptInterface fun setVoiceMode(on: Boolean) {
                    runOnUiThread { applyVoiceMode(on) }
                }
                // Состояние звука для журнала: режим, маршрут, громкости каналов.
                @JavascriptInterface fun audioInfo(): String = describeAudio()
                @JavascriptInterface fun getVersionCode(): Int = BuildConfig.VERSION_CODE
                @JavascriptInterface fun getVersionName(): String = BuildConfig.VERSION_NAME
                // Обновление с GitHub: адрес и SHA-256 приходят из приложения (оно читает их из выпуска на GitHub).
                @JavascriptInterface fun installUpdate(url: String, sha256: String) {
                    val parsed = try { Uri.parse(url) } catch (_: Exception) { null }
                    val host = parsed?.host?.lowercase() ?: ""
                    val allowedHost = host == "github.com" || host.endsWith(".githubusercontent.com")
                    if (parsed?.scheme != "https" || !allowedHost || !Regex("^[0-9a-fA-F]{64}$").matches(sha256)) {
                        toast(text("android.update.address"))
                        return
                    }
                    Thread { downloadAndInstall(url, sha256.lowercase()) }.start()
                }            }, "PreferansAndroid")
            web.webViewClient = object : WebViewClient() {
                override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                    val u = request.url
                    return u.scheme != origin.scheme || u.host != origin.host || u.port != origin.port
                }
            }
            web.webChromeClient = object : WebChromeClient() {
                override fun onPermissionRequest(request: PermissionRequest) {
                    runOnUiThread {
                        val u = request.origin
                        if (u.scheme != origin.scheme || u.host != origin.host || u.port != origin.port ||
                            !request.resources.contains(PermissionRequest.RESOURCE_AUDIO_CAPTURE)) {
                            request.deny()
                        } else if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED) {
                            request.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
                        } else {
                            microphoneRequest?.deny()
                            microphoneRequest = request
                            requestPermissions(arrayOf(Manifest.permission.RECORD_AUDIO), 10)
                        }
                    }
                }
                override fun onPermissionRequestCanceled(request: PermissionRequest) {
                    if (microphoneRequest === request) microphoneRequest = null
                }
                override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
                    files?.onReceiveValue(null)
                    files = callback
                    startActivityForResult(Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
                        addCategory(Intent.CATEGORY_OPENABLE)
                        type = "text/plain"
                    }, 1)
                    return true
                }
            }
            web.setOnApplyWindowInsetsListener { view, insets ->
                view.setPadding(insets.systemWindowInsetLeft, insets.systemWindowInsetTop, insets.systemWindowInsetRight, insets.systemWindowInsetBottom)
                insets
            }
            setContentView(web)
            if (Build.VERSION.SDK_INT >= 33)
                registerReceiver(screenOffReceiver, IntentFilter(Intent.ACTION_SCREEN_OFF), Context.RECEIVER_NOT_EXPORTED)
            else
                registerReceiver(screenOffReceiver, IntentFilter(Intent.ACTION_SCREEN_OFF))
            web.loadUrl(url)
        } catch (e: Exception) {
            setContentView(TextView(this).apply { text = text("android.start.failed", e.message); setPadding(24, 48, 24, 24) })
        }
    }
    // Пока микрофон включён, WebView переводит звук Android в режим связи (громкость канала «голосовой вызов», эхоподавление),
    // а после выключения возвращает обычный: тот же собеседник звучит то тише, то громче. Поэтому на время голосовой связи
    // режим связи и динамик держим сами и возвращаем, если WebView его сбросил.
    @Volatile private var voiceModeOn = false
    private var voiceModeSaved = AudioManager.MODE_NORMAL
    private var voiceModeFixes = 0
    private val audioHandler = Handler(Looper.getMainLooper())
    private val audioGuard = object : Runnable {
        override fun run() {
            if (!voiceModeOn) return
            val am = getSystemService(AUDIO_SERVICE) as AudioManager
            if (am.mode != AudioManager.MODE_IN_COMMUNICATION) {
                am.mode = AudioManager.MODE_IN_COMMUNICATION
                voiceModeFixes++
                routeVoice(am)
            }
            audioHandler.postDelayed(this, 2000)
        }
    }
    private val audioDevices = object : AudioDeviceCallback() {
        override fun onAudioDevicesAdded(added: Array<out AudioDeviceInfo>) { if (voiceModeOn) routeVoice(getSystemService(AUDIO_SERVICE) as AudioManager) }
        override fun onAudioDevicesRemoved(removed: Array<out AudioDeviceInfo>) { if (voiceModeOn) routeVoice(getSystemService(AUDIO_SERVICE) as AudioManager) }
    }
    private fun hasHeadset(am: AudioManager): Boolean = am.getDevices(AudioManager.GET_DEVICES_OUTPUTS).any {
        it.type == AudioDeviceInfo.TYPE_WIRED_HEADSET || it.type == AudioDeviceInfo.TYPE_WIRED_HEADPHONES ||
            it.type == AudioDeviceInfo.TYPE_BLUETOOTH_SCO || it.type == AudioDeviceInfo.TYPE_BLUETOOTH_A2DP ||
            it.type == AudioDeviceInfo.TYPE_USB_HEADSET || it.type == 26 // 26 = BLE-гарнитура (API 31)
    }
    // С наушниками или гарнитурой маршрут выбирает система, иначе звук идёт в динамик, а не в разговорный.
    private fun routeVoice(am: AudioManager) {
        try {
            val headset = hasHeadset(am)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                if (headset) am.clearCommunicationDevice()
                else am.availableCommunicationDevices.firstOrNull { it.type == AudioDeviceInfo.TYPE_BUILTIN_SPEAKER }?.let { am.setCommunicationDevice(it) }
            } else {
                @Suppress("DEPRECATION") run { am.isSpeakerphoneOn = !headset }
            }
        } catch (_: Exception) {}
    }
    private fun applyVoiceMode(on: Boolean) {
        if (on == voiceModeOn) return
        val am = getSystemService(AUDIO_SERVICE) as AudioManager
        voiceModeOn = on
        try {
            if (on) {
                voiceModeSaved = am.mode
                am.mode = AudioManager.MODE_IN_COMMUNICATION
                routeVoice(am)
                am.registerAudioDeviceCallback(audioDevices, audioHandler)
                audioHandler.postDelayed(audioGuard, 2000)
            } else {
                audioHandler.removeCallbacks(audioGuard)
                am.unregisterAudioDeviceCallback(audioDevices)
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) am.clearCommunicationDevice()
                else @Suppress("DEPRECATION") run { am.isSpeakerphoneOn = false }
                am.mode = if (voiceModeSaved == AudioManager.MODE_IN_COMMUNICATION) AudioManager.MODE_NORMAL else voiceModeSaved
            }
        } catch (_: Exception) {}
    }
    private fun audioDeviceName(type: Int): String = when (type) {
        AudioDeviceInfo.TYPE_BUILTIN_EARPIECE -> "разговорный"
        AudioDeviceInfo.TYPE_BUILTIN_SPEAKER -> "динамик"
        AudioDeviceInfo.TYPE_WIRED_HEADSET -> "проводная гарнитура"
        AudioDeviceInfo.TYPE_WIRED_HEADPHONES -> "проводные наушники"
        AudioDeviceInfo.TYPE_BLUETOOTH_SCO -> "Bluetooth-связь"
        AudioDeviceInfo.TYPE_BLUETOOTH_A2DP -> "Bluetooth-музыка"
        AudioDeviceInfo.TYPE_USB_HEADSET -> "USB-гарнитура"
        26 -> "BLE-гарнитура"
        else -> "тип $type"
    }
    private fun describeAudio(): String = try {
        val am = getSystemService(AUDIO_SERVICE) as AudioManager
        val mode = when (am.mode) {
            AudioManager.MODE_NORMAL -> "обычный"
            AudioManager.MODE_IN_COMMUNICATION -> "связь"
            AudioManager.MODE_IN_CALL -> "звонок"
            AudioManager.MODE_RINGTONE -> "рингтон"
            else -> am.mode.toString()
        }
        val route = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) am.communicationDevice?.let { audioDeviceName(it.type) } ?: "по умолчанию"
        else @Suppress("DEPRECATION") run { if (am.isSpeakerphoneOn) "динамик" else "по умолчанию" }
        val outs = am.getDevices(AudioManager.GET_DEVICES_OUTPUTS).joinToString(",") { audioDeviceName(it.type) }
        fun vol(stream: Int) = "${am.getStreamVolume(stream)}/${am.getStreamMaxVolume(stream)}"
        "режим=$mode, фиксация=${if (voiceModeOn) "вкл" else "выкл"} (возвратов режима: $voiceModeFixes), маршрут=$route, выходы=[$outs], " +
            "громкость: связь ${vol(AudioManager.STREAM_VOICE_CALL)}, музыка ${vol(AudioManager.STREAM_MUSIC)}, микрофон выключен системой=${am.isMicrophoneMute}"
    } catch (e: Exception) { "ошибка: ${e.javaClass.simpleName}" }
    private fun startKeepAlive() {
        if (exiting) return
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED &&
            !getSharedPreferences("interface", MODE_PRIVATE).getBoolean("notificationsAsked", false)) {
            // Без разрешения служба работает, но её уведомления не видно: спрашиваем один раз.
            getSharedPreferences("interface", MODE_PRIVATE).edit().putBoolean("notificationsAsked", true).apply()
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 12)
        }
        keepAliveOn = true
        try {
            startForegroundService(Intent(this, KeepAliveService::class.java)
                .putExtra("channel", text("android.keepalive.channel"))
                .putExtra("title", text("android.keepalive.title"))
                .putExtra("text", text("android.keepalive.text")))
        } catch (_: Exception) {}
        askIgnoreBatteryOptimizations()
    }
    private fun stopKeepAlive() {
        keepAliveOn = false
        try { stopService(Intent(this, KeepAliveService::class.java)) } catch (_: Exception) {}
    }
    // Доза и экономия батареи отключают сеть у свёрнутого приложения. Просим исключение один раз за установку.
    private fun askIgnoreBatteryOptimizations() {
        val prefs = getSharedPreferences("interface", MODE_PRIVATE)
        val power = getSystemService(POWER_SERVICE) as android.os.PowerManager
        if (power.isIgnoringBatteryOptimizations(packageName) || prefs.getBoolean("batteryAsked", false)) return
        prefs.edit().putBoolean("batteryAsked", true).apply()
        try {
            startActivity(Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, Uri.parse("package:$packageName")))
        } catch (_: Exception) {}
    }
    private fun toast(message: String) = runOnUiThread {
        android.widget.Toast.makeText(this, message, android.widget.Toast.LENGTH_LONG).show()
    }
    private fun exportLogToDownloads() {
        if (exportingLog) return
        if (Build.VERSION.SDK_INT < 29 && checkSelfPermission(Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.WRITE_EXTERNAL_STORAGE), 11)
            return
        }
        exportingLog = true
        Thread {
            var uri: Uri? = null
            var target: File? = null
            try {
                val source = File(filesDir, "preferans.log")
                if (!source.isFile) throw Exception(text("android.log.missing"))
                val stamp = java.text.SimpleDateFormat("yyyyMMdd-HHmmss-SSS", java.util.Locale.ROOT).format(java.util.Date())
                val name = "preferans-$stamp.log"
                val output = if (Build.VERSION.SDK_INT >= 29) {
                    uri = contentResolver.insert(MediaStore.Downloads.EXTERNAL_CONTENT_URI, ContentValues().apply {
                        put(MediaStore.MediaColumns.DISPLAY_NAME, name)
                        put(MediaStore.MediaColumns.MIME_TYPE, "text/plain")
                        put(MediaStore.MediaColumns.RELATIVE_PATH, Environment.DIRECTORY_DOWNLOADS)
                        put(MediaStore.MediaColumns.IS_PENDING, 1)
                    }) ?: throw Exception(text("android.log.create_failed"))
                    contentResolver.openOutputStream(uri!!) ?: throw Exception(text("android.log.create_failed"))
                } else {
                    target = File(Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_DOWNLOADS), name)
                    target!!.parentFile?.mkdirs()
                    target!!.outputStream()
                }
                output.use { out ->
                    source.inputStream().use { input ->
                        // Bound the copy to its initial size while the live log keeps growing.
                        var remaining = input.channel.size()
                        val buffer = ByteArray(64 * 1024)
                        while (remaining > 0) {
                            val count = input.read(buffer, 0, minOf(remaining, buffer.size.toLong()).toInt())
                            if (count < 0) break
                            out.write(buffer, 0, count)
                            remaining -= count
                        }
                    }
                }
                if (Build.VERSION.SDK_INT >= 29) {
                    contentResolver.update(uri!!, ContentValues().apply { put(MediaStore.MediaColumns.IS_PENDING, 0) }, null, null)
                } else {
                    MediaScannerConnection.scanFile(this, arrayOf(target!!.absolutePath), arrayOf("text/plain"), null)
                }
                toast(text("android.log.saved", name))
            } catch (e: Exception) {
                uri?.let { try { contentResolver.delete(it, null, null) } catch (_: Exception) {} }
                target?.delete()
                toast(text("android.log.failed", e.message ?: text("android.network.error")))
            } finally {
                runOnUiThread { exportingLog = false }
            }
        }.start()
    }
    private fun downloadAndInstall(url: String, expectedSha256: String) {
        var connection: HttpURLConnection? = null
        try {
            toast(text("android.update.downloading"))
            connection = (URL(url).openConnection() as HttpURLConnection).apply {
                connectTimeout = 10000
                readTimeout = 120000
                instanceFollowRedirects = true
                requestMethod = "GET"
            }
            connection.connect()
            if (connection.responseCode !in 200..299) throw Exception("HTTP ${connection.responseCode}")
            if (connection.contentLengthLong > 128L * 1024 * 1024) throw Exception(text("android.update.apk_size"))
            val target = File(cacheDir, "preferans-update.apk")
            val digest = java.security.MessageDigest.getInstance("SHA-256")
            connection.inputStream.use { input ->
                target.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    var total = 0L
                    while (true) {
                        val count = input.read(buffer)
                        if (count < 0) break
                        total += count
                        if (total > 128L * 1024 * 1024) throw Exception(text("android.update.apk_size"))
                        digest.update(buffer, 0, count)
                        output.write(buffer, 0, count)
                    }
                }
            }
            val actualSha256 = digest.digest().joinToString("") { "%02x".format(it) }
            if (actualSha256 != expectedSha256) {
                target.delete()
                throw Exception(text("android.update.hash"))
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
                toast(text("android.update.permission"))
                runOnUiThread { startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:$packageName"))) }
                return
            }
            val uri = Uri.parse("content://$packageName.update-provider/preferans-update.apk")
            runOnUiThread { startActivity(Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, "application/vnd.android.package-archive")
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }) }
        } catch (e: Exception) {
            toast(text("android.update.failed", e.message ?: text("android.network.error")))
        } finally {
            connection?.disconnect()
        }
    }
    @Deprecated("Legacy file chooser callback")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == 1) { files?.onReceiveValue(if (resultCode == RESULT_OK && data?.data != null) arrayOf(data.data!!) else null); files = null }
        if (requestCode == 2) {
            val code = pendingCode
            pendingCode = null
            if (resultCode == RESULT_OK && code != null && data?.data != null) {
                try { contentResolver.openOutputStream(data.data!!)?.use { it.write(code.toByteArray(Charsets.UTF_8)) } }
                catch (e: Exception) { android.widget.Toast.makeText(this, text("android.code.save_failed"), android.widget.Toast.LENGTH_LONG).show() }
            }
        }
    }
    override fun onDestroy() {
        if (isFinishing) stopKeepAlive()
        networkCallback?.let { try { (getSystemService(CONNECTIVITY_SERVICE) as ConnectivityManager).unregisterNetworkCallback(it) } catch (_: Exception) {} }
        try { unregisterReceiver(screenOffReceiver) } catch (_: IllegalArgumentException) { }
        applyVoiceMode(false)
        microphoneRequest?.deny()
        microphoneRequest = null
        files?.onReceiveValue(null)
        if (::web.isInitialized) web.destroy()
        if (isFinishing) Mobile.stop()
        super.onDestroy()
        // Explicit exit also stops gomobile/background threads after normal cleanup.
        if (exiting) android.os.Process.killProcess(android.os.Process.myPid())
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 10) {
            val request = microphoneRequest
            microphoneRequest = null
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) {
                request?.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
                // Служба получает тип «микрофон» только при выданном разрешении: перезапускаем её, если она уже работает.
                if (keepAliveOn) startKeepAlive()
            } else request?.deny()
        }
        if (requestCode == 11) {
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) exportLogToDownloads()
            else toast(text("android.log.permission_denied"))
        }
    }
    override fun onPause() {
        if (::web.isInitialized) {
            val locked = (getSystemService(KEYGUARD_SERVICE) as KeyguardManager).isKeyguardLocked
            val event = if (locked) "preferans-voice-lock" else "preferans-voice-background"
            web.evaluateJavascript("window.dispatchEvent(new Event('$event'))", null)
            // Пока работает служба переднего плана, свёрнутое приложение продолжает разговор: страницу усыпляем только
            // при блокировке устройства (тогда микрофон уже выключен).
            if (locked || !keepAliveOn) web.onPause()
        }
        super.onPause()
    }
    override fun onResume() {
        super.onResume()
        if (::web.isInitialized) {
            web.onResume()
            web.evaluateJavascript("window.dispatchEvent(new Event('preferans-voice-resume'))", null)
        }
        // После блокировки экрана сокеты могли умереть: проверяем соединения и сбрасываем штрафы повторных попыток.
        Mobile.networkChanged()
    }
    // Тип основной сети (Wi-Fi, мобильная, ...) нужен только журналу: по нему видно, что телефон ушёл с домашней Wi-Fi.
    private fun reportNetworkKind(cm: ConnectivityManager, network: Network) {
        try {
            val caps = cm.getNetworkCapabilities(network) ?: return
            val kinds = ArrayList<String>()
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)) kinds.add("wifi")
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)) kinds.add("cellular")
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET)) kinds.add("ethernet")
            if (caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) kinds.add("vpn")
            if (kinds.isEmpty()) kinds.add("other")
            Mobile.setNetworkKind(kinds.joinToString("+"), caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED))
        } catch (_: Exception) {}
    }
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private fun watchNetwork() {
        val cm = getSystemService(CONNECTIVITY_SERVICE) as ConnectivityManager
        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) { reportNetworkKind(cm, network); Mobile.networkChanged() }
            override fun onCapabilitiesChanged(network: Network, caps: NetworkCapabilities) { reportNetworkKind(cm, network) }
            override fun onLost(network: Network) { try { Mobile.setNetworkKind("none", false) } catch (_: Exception) {} }
        }
        try { cm.registerDefaultNetworkCallback(callback); networkCallback = callback } catch (_: Exception) {}
    }
}
