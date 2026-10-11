package id.co.erayadigital.arus_mobile

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
