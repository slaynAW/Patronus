import org.jetbrains.kotlin.gradle.dsl.JvmTarget

// Module Kotlin pur (sans dépendance Android) : toute la logique métier y est
// regroupée pour être testée rapidement sur la JVM et réutilisée par l'app.
plugins {
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
        allWarningsAsErrors.set(true)
    }
}

dependencies {
    api(libs.kotlinx.coroutines.core)
    api(libs.kotlinx.serialization.json)

    testImplementation(platform(libs.junit.bom))
    testImplementation(libs.junit.jupiter)
    testImplementation(libs.kotlinx.coroutines.test)
    testRuntimeOnly(libs.junit.platform.launcher)
}

tasks.test {
    useJUnitPlatform()
    // Les vecteurs de test partagés avec l'agent Go sont à la racine du dépôt.
    systemProperty("wol.protocolDir", file("../protocol").absolutePath)
    // Client GitHub du partage : la JVM refuse la méthode PATCH (Android l'accepte), fixée par réflexion.
    jvmArgs("--add-opens=java.base/java.net=ALL-UNNAMED")
    testLogging {
        events("failed")
        exceptionFormat = org.gradle.api.tasks.testing.logging.TestExceptionFormat.FULL
    }
}
