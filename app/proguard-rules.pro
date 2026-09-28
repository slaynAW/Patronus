# Règles R8 spécifiques à l'application.
# kotlinx.serialization embarque ses propres règles ; on conserve en plus les classes du modèle
# (le format JSON des sauvegardes est un contrat durable).
-keep class io.github.slaynaw.wakeonlan.core.model.** { *; }
-keep class io.github.slaynaw.wakeonlan.core.config.ExportEnvelope { *; }
-keep class io.github.slaynaw.wakeonlan.core.config.EncryptionInfo { *; }
# Partage : format des fichiers d'accès (commun avec Windows) et état enregistré.
-keep class io.github.slaynaw.wakeonlan.core.share.** { *; }

# Messages d'erreur plus lisibles dans les rapports de plantage.
-keepattributes SourceFile,LineNumberTable
-renamesourcefileattribute SourceFile

# Pas d'obfuscation : pour une application personnelle, des rapports de
# plantage lisibles (noms de classes réels) valent plus qu'un APK légèrement plus petit.
-dontobfuscate

# Google code scanner (ML Kit) : ses composants sont instanciés par réflexion à partir du
# manifeste. Les règles fournies par la bibliothèque (2023) ne conservent pas explicitement
# leurs constructeurs, ce que le mode R8 strict d'AGP 9 exige : sans ces lignes, le scan du
# QR code fait planter l'APK de publication. Contrôlé en CI (.github/scripts/check-r8-registrars.sh).
-keep class * implements com.google.firebase.components.ComponentRegistrar { <init>(); }
-keep class com.google.mlkit.** { <init>(...); }
