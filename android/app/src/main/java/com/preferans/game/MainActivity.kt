package com.preferans.game

import android.app.Activity
import android.Manifest
import android.content.pm.PackageManager
import android.webkit.PermissionRequest
import android.content.Intent
import android.content.ContentValues
import android.os.Environment
import android.provider.MediaStore
import android.media.MediaScannerConnection
import android.content.pm.ApplicationInfo
import android.net.Uri
import android.os.Bundle
import android.os.Build
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
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        language = getSharedPreferences("interface", MODE_PRIVATE).getString("language", "uk") ?: "uk"
        try {
            Mobile.setSecretCipher(ApiKeyCipher())
            val url = if (BuildConfig.NETWORK_TEST) Mobile.startNetworkTest(filesDir.absolutePath) else Mobile.start(filesDir.absolutePath)
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
                @JavascriptInterface fun saveReport(report: String) {
                    if (!BuildConfig.NETWORK_TEST || report.length > 1048576) return
                    runOnUiThread {
                        pendingCode = report
                        startActivityForResult(Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
                            addCategory(Intent.CATEGORY_OPENABLE)
                            type = "application/json"
                            putExtra(Intent.EXTRA_TITLE, "preferans-network-report.json")
                        }, 2)
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
                @JavascriptInterface fun getVersionCode(): Int = BuildConfig.VERSION_CODE
                @JavascriptInterface fun getVersionName(): String = BuildConfig.VERSION_NAME
                @JavascriptInterface fun checkUpdate(url: String, requestId: Int) {
                    if (requestId < 1) return
                    Thread {
                        var connection: HttpURLConnection? = null
                        val result = try {
                            val base = URL(url)
                            if (base.protocol !in setOf("http", "https")) throw Exception(text("android.update.protocol"))
                            val manifestURL = URL(url.trimEnd('/') + "/version.json?t=" + System.currentTimeMillis())
                            connection = (manifestURL.openConnection() as HttpURLConnection).apply {
                                connectTimeout = 10000
                                readTimeout = 15000
                                instanceFollowRedirects = true
                                requestMethod = "GET"
                            }
                            connection!!.connect()
                            if (connection!!.responseCode !in 200..299) throw Exception(text("android.update.http", connection!!.responseCode))
                            val bytes = connection!!.inputStream.use { input -> input.readBytes() }
                            if (bytes.size > 1024 * 1024) throw Exception(text("android.update.manifest_size"))
                            Pair(true, String(bytes, Charsets.UTF_8))
                        } catch (e: Exception) {
                            Pair(false, e.message ?: text("android.network.error"))
                        } finally {
                            connection?.disconnect()
                        }
                        val payload = JSONObject.quote(result.second)
                        runOnUiThread {
                            if (::web.isInitialized) web.evaluateJavascript("window.__preferansUpdateResult($requestId,${result.first},$payload)", null)
                        }
                    }.start()
                }
                @JavascriptInterface fun installUpdate(url: String) {
                    val parsed = try { Uri.parse(url) } catch (_: Exception) { null }
                    if (parsed?.scheme !in setOf("http", "https")) {
                        toast(text("android.update.address"))
                        return
                    }
                    Thread { downloadAndInstall(url) }.start()
                }
            }, "PreferansAndroid")
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
            web.loadUrl(url)
        } catch (e: Exception) {
            setContentView(TextView(this).apply { text = text("android.start.failed", e.message); setPadding(24, 48, 24, 24) })
        }
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
    private fun downloadAndInstall(url: String) {
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
            connection.inputStream.use { input ->
                target.outputStream().use { output ->
                    val buffer = ByteArray(64 * 1024)
                    var total = 0L
                    while (true) {
                        val count = input.read(buffer)
                        if (count < 0) break
                        total += count
                        if (total > 128L * 1024 * 1024) throw Exception(text("android.update.apk_size"))
                        output.write(buffer, 0, count)
                    }
                }
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
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED)
                request?.grant(arrayOf(PermissionRequest.RESOURCE_AUDIO_CAPTURE))
            else request?.deny()
        }
        if (requestCode == 11) {
            if (grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED) exportLogToDownloads()
            else toast(text("android.log.permission_denied"))
        }
    }
    override fun onPause() {
        if (::web.isInitialized) {
            web.evaluateJavascript("window.dispatchEvent(new Event('preferans-voice-suspend'))", null)
            web.onPause()
        }
        super.onPause()
    }
    override fun onResume() {
        super.onResume()
        if (::web.isInitialized) web.onResume()
    }
}
