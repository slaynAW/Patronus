package io.github.slaynaw.wakeonlan.core.share

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.ReceiveChannel
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.io.InputStream
import java.net.HttpURLConnection
import java.net.ProtocolException
import java.net.URI
import java.net.URLEncoder

/** Code de connexion à saisir sur github.com. */
@Serializable
data class GitHubDeviceCode(
    @SerialName("device_code") val deviceCode: String = "",
    @SerialName("user_code") val userCode: String = "",
    @SerialName("verification_uri") val verificationUri: String = "",
    @SerialName("expires_in") val expiresIn: Int = 900,
    val interval: Int = 5,
)

/** Contenu d'un Gist lu sans compte. */
data class GistSnapshot(val notModified: Boolean, val etag: String, val files: Map<String, String>)

/** Fichier d'un Gist (sauvegardes). */
data class GistFile(val name: String, val size: Int, val content: String)

/** Gist du compte (identifiant et description). */
data class GistInfo(val id: String, val description: String)

/**
 * Client minimal de l'API GitHub pour le partage (même comportement que l'application Windows) :
 * connexion par code (droit « gist » uniquement), Gist secret de la personne qui partage, lecture
 * publique des fichiers d'accès (chiffrés) avec ETag.
 */
class ShareGitHub(
    val clientId: String,
    private val userAgent: String = "Patronus",
    private val api: String = "https://api.github.com",
    private val web: String = "https://github.com",
    /** Unité des délais de la connexion (1 s ; raccourcie dans les tests). */
    private val pollUnitMillis: Long = 1000,
) {
    private val json = Json { ignoreUnknownKeys = true }

    /** Demande un code de connexion. */
    suspend fun startLogin(): GitHubDeviceCode = withContext(Dispatchers.IO) {
        if (clientId.isEmpty()) throw ShareException(ShareException.Reason.UNAUTHORIZED, "connexion GitHub non configurée dans cette version")
        val body = form("$web/login/device/code", "client_id" to clientId, "scope" to "gist")
        val code = decode(body) { json.decodeFromString(GitHubDeviceCode.serializer(), it) }
        if (code.deviceCode.isEmpty() || code.userCode.isEmpty()) unexpected()
        code.copy(
            interval = code.interval.coerceAtLeast(1),
            verificationUri = code.verificationUri.ifEmpty { "$web/login/device" },
        )
    }

    /**
     * Attend la validation du code sur github.com et renvoie le jeton d'accès.
     *
     * Les erreurs réseau ne sont pas fatales tant que le code est valable : pendant la validation dans le
     * navigateur, l'application passe en arrière-plan et le système peut lui couper l'accès à Internet.
     * [wake] permet d'interroger GitHub tout de suite (retour dans l'application).
     */
    suspend fun waitLogin(code: GitHubDeviceCode, wake: ReceiveChannel<Unit>? = null): String {
        var interval = code.interval.toLong() * pollUnitMillis
        val deadline = System.currentTimeMillis() + code.expiresIn.coerceAtLeast(60) * pollUnitMillis
        var answered = System.currentTimeMillis()
        while (true) {
            if (wake == null) delay(interval) else withTimeoutOrNull(interval) { wake.receive() }
            // GitHub ralentit (« slow_down ») les applications qui l'interrogent plus souvent que demandé.
            val early = answered + interval - System.currentTimeMillis()
            if (early > 0) delay(early)
            val r = try {
                val body = withContext(Dispatchers.IO) {
                    form(
                        "$web/login/oauth/access_token",
                        "client_id" to clientId,
                        "device_code" to code.deviceCode,
                        "grant_type" to "urn:ietf:params:oauth:grant-type:device_code",
                    )
                }
                decode(body) { json.parseToJsonElement(it).jsonObject }
            } catch (e: ShareException) {
                if (!e.isTransient || System.currentTimeMillis() > deadline) throw e
                continue
            }
            answered = System.currentTimeMillis()
            val token = r["access_token"]?.jsonPrimitive?.contentOrNull
            when (val error = r["error"]?.jsonPrimitive?.contentOrNull) {
                null -> return token?.takeIf { it.isNotEmpty() } ?: unexpected()
                "authorization_pending" -> Unit
                "slow_down" -> interval = (r["interval"]?.jsonPrimitive?.intOrNull?.toLong()?.times(pollUnitMillis)) ?: (interval + 5 * pollUnitMillis)
                "access_denied" -> throw ShareException(ShareException.Reason.DENIED, "connexion refusée sur GitHub")
                "expired_token" -> throw ShareException(ShareException.Reason.DENIED, "code expiré : recommencez la connexion")
                else -> throw ShareException(ShareException.Reason.NETWORK, "connexion GitHub impossible ($error)")
            }
            if (System.currentTimeMillis() > deadline) throw ShareException(ShareException.Reason.DENIED, "code expiré : recommencez la connexion")
        }
    }

    /** Identifiant GitHub du compte connecté. */
    suspend fun user(token: String): String = withContext(Dispatchers.IO) {
        val r = request("GET", "/user", token).json()
        r["login"]?.jsonPrimitive?.contentOrNull?.takeIf { ShareLinks.LOGIN.matches(it) } ?: unexpected()
    }

    /** Crée un Gist secret et renvoie son identifiant. */
    suspend fun createGist(token: String, description: String, files: Map<String, String>): String = withContext(Dispatchers.IO) {
        val body = buildJsonObject {
            put("description", description)
            put("public", false)
            put("files", JsonObject(files.mapValues { (_, content) -> buildJsonObject { put("content", content) } }))
        }
        val r = request("POST", "/gists", token, body = body.toString()).json()
        r["id"]?.jsonPrimitive?.contentOrNull?.takeIf { ShareLinks.GIST.matches(it) } ?: unexpected()
    }

    /** Ajoute, remplace (contenu) ou supprime (null) des fichiers d'un Gist. */
    suspend fun updateGist(token: String, id: String, files: Map<String, String?>) = withContext(Dispatchers.IO) {
        checkGist(id)
        val body = buildJsonObject {
            put("files", JsonObject(files.mapValues { (_, content) -> content?.let { buildJsonObject { put("content", it) } } ?: JsonNull }))
        }
        request("PATCH", "/gists/$id", token, body = body.toString())
        Unit
    }

    /** Supprime un Gist. */
    suspend fun deleteGist(token: String, id: String) = withContext(Dispatchers.IO) {
        checkGist(id)
        request("DELETE", "/gists/$id", token)
        Unit
    }

    /** Lit les fichiers d'accès d'un Gist, sans jeton (contenu chiffré). */
    suspend fun fetchGist(id: String, etag: String): GistSnapshot = withContext(Dispatchers.IO) {
        checkGist(id)
        val r = request("GET", "/gists/$id", null, etag = etag)
        if (r.code == HttpURLConnection.HTTP_NOT_MODIFIED) return@withContext GistSnapshot(true, etag, emptyMap())
        val files = HashMap<String, String>()
        (r.json()["files"] as? JsonObject)?.forEach { (name, value) ->
            if (!name.startsWith("acces-")) return@forEach
            val f = value as? JsonObject ?: return@forEach
            val truncated = (f["truncated"] as? JsonPrimitive)?.boolean ?: false
            files[name] = if (truncated) raw(f["raw_url"]?.jsonPrimitive?.contentOrNull.orEmpty(), ShareCrypto.MAX_FILE_SIZE) else f["content"]?.jsonPrimitive?.contentOrNull.orEmpty()
        }
        GistSnapshot(false, r.etag, files)
    }

    /** Identifiant du Gist du compte portant cette [description] (sauvegardes), ou null. */
    suspend fun findGist(token: String, description: String): String? = withContext(Dispatchers.IO) {
        for (page in 1..5) {
            val r = request("GET", "/gists?per_page=100&page=$page", token, limit = 8 shl 20)
            val list = decode(r.body) { Json.parseToJsonElement(it) as? JsonArray } ?: unexpected()
            for (item in list) {
                val o = item as? JsonObject ?: continue
                val id = o["id"]?.jsonPrimitive?.contentOrNull ?: continue
                if (o["description"]?.jsonPrimitive?.contentOrNull == description && ShareLinks.GIST.matches(id)) return@withContext id
            }
            if (list.size < 100) break
        }
        null
    }

    /** Gists du compte dont la description commence par [prefix] (500 examinés au plus, archives). */
    suspend fun listGists(token: String, prefix: String): List<GistInfo> = withContext(Dispatchers.IO) {
        val out = ArrayList<GistInfo>()
        for (page in 1..5) {
            val r = request("GET", "/gists?per_page=100&page=$page", token, limit = 8 shl 20)
            val list = decode(r.body) { Json.parseToJsonElement(it) as? JsonArray } ?: unexpected()
            for (item in list) {
                val o = item as? JsonObject ?: continue
                val id = o["id"]?.jsonPrimitive?.contentOrNull ?: continue
                val description = o["description"]?.jsonPrimitive?.contentOrNull ?: continue
                if (description.startsWith(prefix) && ShareLinks.GIST.matches(id)) out += GistInfo(id, description)
            }
            if (list.size < 100) break
        }
        out
    }

    /**
     * Lit tous les fichiers d'un Gist avec le jeton ; les fichiers tronqués par l'API sont relus en
     * entier ([limit] octets au plus chacun, les plus gros sont ignorés).
     */
    suspend fun readGist(token: String, id: String, limit: Int): List<GistFile> = readGist(token, id, limit, strict = false)

    /** Comme [readGist], mais échoue si un fichier dépasse [limit] (pour recopier un Gist sans rien perdre). */
    suspend fun readGistAll(token: String, id: String, limit: Int): List<GistFile> = readGist(token, id, limit, strict = true)

    private suspend fun readGist(token: String, id: String, limit: Int, strict: Boolean): List<GistFile> = withContext(Dispatchers.IO) {
        checkGist(id)
        val r = request("GET", "/gists/$id", token, limit = 32 shl 20)
        val files = ArrayList<GistFile>()
        (r.json()["files"] as? JsonObject)?.forEach { (name, value) ->
            val f = value as? JsonObject ?: return@forEach
            val size = f["size"]?.jsonPrimitive?.intOrNull ?: 0
            if (size > limit) {
                if (strict) throw ShareException(ShareException.Reason.INVALID, "fichier $name trop volumineux ($size octets)")
                return@forEach
            }
            val truncated = (f["truncated"] as? JsonPrimitive)?.boolean ?: false
            val content = if (truncated) raw(f["raw_url"]?.jsonPrimitive?.contentOrNull.orEmpty(), limit) else f["content"]?.jsonPrimitive?.contentOrNull.orEmpty()
            files += GistFile(name, size, content)
        }
        files.sortedBy { it.name }
    }

    // --- HTTP ---

    private class Response(val code: Int, val body: String, val etag: String) {
        fun json(): JsonObject = try {
            Json.parseToJsonElement(body).jsonObject
        } catch (e: SerializationException) {
            throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub illisible", e)
        } catch (e: IllegalArgumentException) {
            throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub illisible", e)
        }
    }

    private fun checkGist(id: String) {
        if (!ShareLinks.GIST.matches(id)) throw ShareException(ShareException.Reason.NOT_FOUND, "espace de partage introuvable")
    }

    private fun open(url: String, method: String): HttpURLConnection =
        (URI(url).toURL().openConnection() as HttpURLConnection).apply {
            connectTimeout = 15_000
            readTimeout = 20_000
            setRequestProperty("User-Agent", userAgent)
            setMethod(this, method)
        }

    private fun form(url: String, vararg values: Pair<String, String>): String {
        val connection = open(url, "POST")
        try {
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/x-www-form-urlencoded")
            connection.setRequestProperty("Accept", "application/json")
            val body = values.joinToString("&") { (k, v) -> k + "=" + URLEncoder.encode(v, "UTF-8") }
            connection.outputStream.use { it.write(body.toByteArray()) }
            val code = connection.responseCode
            if (code != HttpURLConnection.HTTP_OK) throw status(code)
            return read(connection.inputStream, 1 shl 20)
        } catch (e: ShareException) {
            throw e
        } catch (e: IOException) {
            throw network(e)
        } finally {
            connection.disconnect()
        }
    }

    private fun request(method: String, path: String, token: String?, body: String? = null, etag: String = "", limit: Int = 1 shl 20): Response {
        val connection = open(api + path, method)
        try {
            connection.setRequestProperty("Accept", "application/vnd.github+json")
            connection.setRequestProperty("X-GitHub-Api-Version", "2022-11-28")
            if (token != null) connection.setRequestProperty("Authorization", "Bearer $token")
            if (etag.isNotEmpty()) connection.setRequestProperty("If-None-Match", etag)
            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.outputStream.use { it.write(body.toByteArray()) }
            }
            val code = connection.responseCode
            val limited = connection.getHeaderField("X-RateLimit-Remaining") == "0"
            when {
                code == HttpURLConnection.HTTP_NOT_MODIFIED -> return Response(code, "", etag)
                code == HttpURLConnection.HTTP_NOT_FOUND -> throw ShareException(ShareException.Reason.NOT_FOUND, "espace de partage introuvable")
                code == HttpURLConnection.HTTP_UNAUTHORIZED -> throw unauthorized()
                code == 429 || (code == HttpURLConnection.HTTP_FORBIDDEN && (token == null || limited)) ->
                    throw ShareException(ShareException.Reason.RATE_LIMITED, "GitHub limite temporairement les requêtes : réessayez dans quelques minutes")
                code == HttpURLConnection.HTTP_FORBIDDEN -> throw unauthorized()
                code !in 200..299 -> throw status(code)
            }
            val text = if (code == HttpURLConnection.HTTP_NO_CONTENT) "" else read(connection.inputStream, limit)
            return Response(code, text, connection.getHeaderField("ETag").orEmpty())
        } catch (e: ShareException) {
            throw e
        } catch (e: IOException) {
            throw network(e)
        } finally {
            connection.disconnect()
        }
    }

    /** Fichier tronqué par l'API : relu seulement depuis le domaine des Gists ([limit] octets au plus). */
    private fun raw(url: String, limit: Int): String {
        val uri = try {
            URI(url)
        } catch (e: IllegalArgumentException) {
            unexpected()
        }
        if (uri.scheme != "https" || uri.host != "gist.githubusercontent.com") unexpected()
        val connection = open(url, "GET")
        try {
            if (connection.responseCode != HttpURLConnection.HTTP_OK) throw status(connection.responseCode)
            return read(connection.inputStream, limit)
        } catch (e: ShareException) {
            throw e
        } catch (e: IOException) {
            throw network(e)
        } finally {
            connection.disconnect()
        }
    }

    private fun read(input: InputStream, limit: Int): String {
        val out = ByteArrayOutputStream()
        input.use {
            val buffer = ByteArray(8 * 1024)
            while (true) {
                val n = it.read(buffer)
                if (n < 0) break
                out.write(buffer, 0, n)
                if (out.size() > limit) throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub trop volumineuse")
            }
        }
        return out.toString("UTF-8")
    }

    private inline fun <T> decode(body: String, parse: (String) -> T): T = try {
        parse(body)
    } catch (e: SerializationException) {
        throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub illisible", e)
    } catch (e: IllegalArgumentException) {
        throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub illisible", e)
    }

    /** Réponse d'erreur : passagère si GitHub est indisponible (5xx) ou surchargé (429). */
    private fun status(code: Int) = ShareException(
        ShareException.Reason.NETWORK,
        "GitHub a répondu $code",
        if (code >= 500 || code == 429) IOException("HTTP $code") else null,
    )

    private fun unauthorized() = ShareException(ShareException.Reason.UNAUTHORIZED, "connexion GitHub expirée ou révoquée : reconnectez-vous")

    private fun network(e: IOException) = ShareException(ShareException.Reason.NETWORK, "GitHub injoignable : vérifiez la connexion Internet", e)

    private fun unexpected(): Nothing = throw ShareException(ShareException.Reason.NETWORK, "réponse de GitHub inattendue")

    private companion object {
        /**
         * PATCH : accepté par HttpURLConnection d'Android (OkHttp), refusé par celui de la JVM (tests),
         * où la méthode est alors fixée par réflexion.
         */
        fun setMethod(connection: HttpURLConnection, method: String) {
            try {
                connection.requestMethod = method
            } catch (e: ProtocolException) {
                val field = HttpURLConnection::class.java.getDeclaredField("method")
                field.isAccessible = true
                field.set(connection, method)
            }
        }
    }
}
