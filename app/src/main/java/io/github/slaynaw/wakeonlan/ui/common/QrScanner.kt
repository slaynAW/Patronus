package io.github.slaynaw.wakeonlan.ui.common

import android.content.Context
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

/**
 * Ouvre le lecteur de QR code de Google Play Services. L'application n'a pas besoin de la
 * permission caméra : c'est Play Services qui gère la caméra, et seul le texte du QR code revient.
 */
fun scanQrCode(context: Context, onResult: (String) -> Unit, onError: (Exception) -> Unit) {
    val options = GmsBarcodeScannerOptions.Builder()
        .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
        .build()
    GmsBarcodeScanning.getClient(context, options)
        .startScan()
        .addOnSuccessListener { barcode -> barcode.rawValue?.let(onResult) }
        .addOnFailureListener { e -> onError(e) }
}
