package io.github.slaynaw.wakeonlan.ui.common

import android.content.Context
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

/** Échec de lecture du QR code. */
sealed interface ScanFailure {
    /** Le module de lecture de Google Play Services est en cours de téléchargement. */
    data object ModuleDownloading : ScanFailure

    data class Error(val message: String) : ScanFailure
}

/**
 * Ouvre le lecteur de QR code de Google Play Services. L'application n'a pas besoin de la
 * permission caméra : c'est Play Services qui gère la caméra, et seul le texte du QR code revient.
 *
 * On vérifie d'abord que le module de lecture est installé : sinon, Play Services lève une
 * erreur impossible à intercepter qui ferme l'application.
 */
fun scanQrCode(context: Context, onResult: (String) -> Unit, onError: (ScanFailure) -> Unit) {
    val fail = { e: Throwable -> onError(ScanFailure.Error(e.localizedMessage ?: e.javaClass.simpleName)) }
    try {
        val options = GmsBarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .build()
        val scanner = GmsBarcodeScanning.getClient(context, options)
        val modules = ModuleInstall.getClient(context)
        modules.areModulesAvailable(scanner)
            .addOnSuccessListener { availability ->
                if (availability.areModulesAvailable()) {
                    scanner.startScan()
                        .addOnSuccessListener { barcode ->
                            barcode.rawValue?.let { value ->
                                try {
                                    onResult(value)
                                } catch (e: Exception) {
                                    fail(e)
                                }
                            }
                        }
                        .addOnFailureListener { e -> fail(e) }
                } else {
                    modules.installModules(ModuleInstallRequest.newBuilder().addApi(scanner).build())
                    onError(ScanFailure.ModuleDownloading)
                }
            }
            .addOnFailureListener { e -> fail(e) }
    } catch (e: Exception) {
        fail(e)
    }
}
