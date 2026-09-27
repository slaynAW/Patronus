# Règles R8 spécifiques à l'application.
# kotlinx.serialization embarque ses propres règles ; on conserve en plus les classes du modèle
# (le format JSON des sauvegardes est un contrat durable).
-keep class io.github.slaynaw.wakeonlan.core.model.** { *; }
-keep class io.github.slaynaw.wakeonlan.core.config.ExportEnvelope { *; }
-keep class io.github.slaynaw.wakeonlan.core.config.EncryptionInfo { *; }

# Messages d'erreur plus lisibles dans les rapports de plantage.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile
