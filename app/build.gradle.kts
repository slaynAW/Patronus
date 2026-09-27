plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

// Version : la CI fournit le nom (tag ou build de développement) et un code croissant (numéro de
// build) pour que chaque nouvel APK s'installe par-dessus le précédent. En local : gradle.properties.
val appVersionName = providers.environmentVariable("WOL_VERSION_NAME").orNull
    ?: providers.gradleProperty("wol.versionName").get()
val appVersionCode = providers.environmentVariable("WOL_VERSION_CODE").orNull?.toIntOrNull() ?: 1

// Signature : la clé n'est JAMAIS dans le dépôt. La CI la reconstitue depuis les secrets GitHub
// (voir docs/SIGNATURE.md). Sans clé, l'APK de release est signé avec la clé de debug locale.
val keystorePath = providers.environmentVariable("WOL_KEYSTORE_FILE").orNull
val keystorePassword = providers.environmentVariable("WOL_KEYSTORE_PASSWORD").orNull?.trim()?.takeIf { it.isNotEmpty() }
val keyAliasName = providers.environmentVariable("WOL_KEY_ALIAS").orNull?.takeIf { it.isNotBlank() } ?: "wakeonlan"

android {
    namespace = "io.github.slaynaw.wakeonlan"
    compileSdk = libs.versions.compileSdk.get().toInt()

    defaultConfig {
        applicationId = "io.github.slaynaw.wakeonlan"
        minSdk = libs.versions.minSdk.get().toInt()
        targetSdk = libs.versions.targetSdk.get().toInt()
        versionCode = appVersionCode
        versionName = appVersionName
    }

    signingConfigs {
        if (keystorePath != null && keystorePassword != null) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = keystorePassword
                keyAlias = keyAliasName
                keyPassword = keystorePassword
                enableV1Signing = false
                enableV2Signing = true
                enableV3Signing = true
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }

    lint {
        abortOnError = true
        checkReleaseBuilds = true
        // Rapport texte (build/reports/lint-results-release.txt), affiché par la CI.
        textReport = true
        // Les mises à jour de dépendances sont gérées manuellement (catalogue de versions).
        disable += setOf("GradleDependency", "NewerVersionAvailable", "AndroidGradlePluginVersion")
    }

    // Pas de bloc de métadonnées de dépendances chiffré par Google dans l'APK (inutile hors Play Store).
    dependenciesInfo {
        includeInApk = false
        includeInBundle = false
    }
}

dependencies {
    implementation(project(":core"))

    implementation(libs.kotlinx.coroutines.android)
    implementation(libs.androidx.core)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.datastore)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons.core)
    debugImplementation(libs.compose.ui.tooling)

    // Lecture du QR code d'appairage via Google Play Services : aucune permission caméra requise.
    implementation(libs.play.services.code.scanner)
}
