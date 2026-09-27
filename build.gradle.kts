// Les plugins sont déclarés une seule fois ici (apply false) pour qu'ils soient chargés
// dans le même classloader, puis appliqués dans chaque module.
plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
}
