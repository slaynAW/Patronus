// Inventaire des composants (SBOM CycloneDX) de l'APK : bibliothèques réellement embarquées dans la
// variante de publication (releaseRuntimeClasspath), telles que Gradle les a résolues.
//
// Appliqué par script d'initialisation dans le job « Dépendances » de la CI
// (./gradlew --init-script .github/scripts/sbom-android.init.gradle.kts :app:cyclonedxDirectBom) :
// le greffon n'est jamais chargé quand l'APK est compilé. Version épinglée ici, à relever à la main
// (Dependabot ne suit pas ce fichier).
import org.cyclonedx.gradle.CyclonedxDirectTask
import org.cyclonedx.gradle.CyclonedxPlugin
import org.cyclonedx.model.Component

initscript {
    repositories {
        gradlePluginPortal()
    }
    dependencies {
        classpath("org.cyclonedx.bom:org.cyclonedx.bom.gradle.plugin:3.4.1")
    }
}

gradle.beforeProject {
    if (path == ":app") {
        apply<CyclonedxPlugin>()
        tasks.named<CyclonedxDirectTask>("cyclonedxDirectBom") {
            includeConfigs = listOf("releaseRuntimeClasspath")
            projectType = Component.Type.APPLICATION
            componentName = "Patronus"
            providers.environmentVariable("WOL_VERSION_NAME").orNull?.let { componentVersion = it }
            jsonOutput = layout.buildDirectory.file("reports/sbom/android.cdx.json")
            xmlOutput.convention(null as RegularFile?)
        }
    }
}
