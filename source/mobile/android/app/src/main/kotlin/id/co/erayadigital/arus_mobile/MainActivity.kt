package id.co.erayadigital.arus_mobile

import android.app.DownloadManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.content.FileProvider
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        // Pembaruan aplikasi: Dart mengunduh APK ke cache, sisi native hanya memberi info versi dan membuka pemasang sistem.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "id.co.erayadigital.arus/updater")
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "info" -> {
                        val p = packageManager.getPackageInfo(packageName, 0)
                        val code = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) p.longVersionCode else @Suppress("DEPRECATION") p.versionCode.toLong()
                        result.success(mapOf("versionName" to (p.versionName ?: ""), "versionCode" to code, "cacheDir" to cacheDir.absolutePath))
                    }
                    // Unduhan lewat DownloadManager sistem: tetap jalan saat layar tidur/aplikasi ditutup, lanjut otomatis (Range) bila putus.
                    "download" -> {
                        val url = call.argument<String>("url")
                        val dir = getExternalFilesDir(null)
                        if (url == null || dir == null) {
                            result.error("ARG", "url/dir", null)
                        } else {
                            val file = File(dir, "arus-update.apk")
                            file.delete()
                            val req = DownloadManager.Request(Uri.parse(url))
                                .setTitle("Pembaruan ARUS")
                                .setDescription("Ketuk untuk memasang")
                                .setMimeType("application/vnd.android.package-archive")
                                .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                                .setDestinationUri(Uri.fromFile(file))
                            if (call.argument<Boolean>("wifiOnly") == true) req.setAllowedNetworkTypes(DownloadManager.Request.NETWORK_WIFI)
                            call.argument<Map<String, String>>("headers")?.forEach { (k, v) -> req.addRequestHeader(k, v) }
                            val id = (getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager).enqueue(req)
                            result.success(mapOf("id" to id, "path" to file.absolutePath))
                        }
                    }
                    "downloadStatus" -> {
                        val id = call.argument<Number>("id")?.toLong() ?: -1L
                        val dm = getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager
                        dm.query(DownloadManager.Query().setFilterById(id)).use { c ->
                            if (!c.moveToFirst()) {
                                result.success(mapOf("state" to "missing", "got" to 0L, "total" to 0L))
                            } else {
                                val state = when (c.getInt(c.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS))) {
                                    DownloadManager.STATUS_SUCCESSFUL -> "done"
                                    DownloadManager.STATUS_FAILED -> "failed"
                                    DownloadManager.STATUS_PAUSED -> "paused"
                                    DownloadManager.STATUS_PENDING -> "pending"
                                    else -> "running"
                                }
                                result.success(mapOf(
                                    "state" to state,
                                    "got" to c.getLong(c.getColumnIndexOrThrow(DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR)),
                                    "total" to c.getLong(c.getColumnIndexOrThrow(DownloadManager.COLUMN_TOTAL_SIZE_BYTES)),
                                ))
                            }
                        }
                    }
                    "downloadRemove" -> {
                        val id = call.argument<Number>("id")?.toLong() ?: -1L
                        (getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager).remove(id)
                        result.success(null)
                    }
                    "install" -> {
                        val path = call.argument<String>("path")
                        if (path == null) {
                            result.error("ARG", "path", null)
                        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
                            // Izin "pasang aplikasi dari sumber ini" belum diberikan: buka layar pengaturannya.
                            startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:$packageName")))
                            result.success("needs_permission")
                        } else {
                            val uri = FileProvider.getUriForFile(this, "$packageName.fileprovider", File(path))
                            val i = Intent(Intent.ACTION_VIEW)
                                .setDataAndType(uri, "application/vnd.android.package-archive")
                                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
                            startActivity(i)
                            result.success("started")
                        }
                    }
                    else -> result.notImplemented()
                }
            }
    }
}
