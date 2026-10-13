package io.github.slaynaw.wakeonlan.core.agent2

import java.io.IOException
import java.io.InputStream
import java.io.OutputStream

/** Trame refusée (§4.1) : dans les trois cas, la connexion est fermée sans réponse. */
class FrameException(val kind: Kind) : IOException(
    when (kind) {
        Kind.EMPTY -> "trame vide"
        Kind.TOO_LARGE -> "trame trop longue"
        Kind.TRUNCATED -> "trame tronquée"
    },
) {
    enum class Kind { EMPTY, TOO_LARGE, TRUNCATED }
}

/** Trames (§4.1) : longueur sur 4 octets (entier non signé, gros-boutiste), puis le message. */
object Frames {
    /**
     * Lit une trame d'au plus [limit] octets ; null si le flux se termine proprement avant elle. La longueur est
     * vérifiée avant toute allocation.
     *
     * @throws FrameException
     */
    fun read(input: InputStream, limit: Int): ByteArray? {
        val header = ByteArray(4)
        val got = readFully(input, header)
        if (got == 0) return null
        if (got < header.size) throw FrameException(FrameException.Kind.TRUNCATED)
        var length = 0L
        for (b in header) length = (length shl 8) or (b.toLong() and 0xFF)
        if (length == 0L) throw FrameException(FrameException.Kind.EMPTY)
        if (length > limit) throw FrameException(FrameException.Kind.TOO_LARGE)
        val payload = ByteArray(length.toInt())
        if (readFully(input, payload) < payload.size) throw FrameException(FrameException.Kind.TRUNCATED)
        return payload
    }

    /** Écrit une trame d'au plus [limit] octets, en un seul envoi. */
    fun write(output: OutputStream, payload: ByteArray, limit: Int) {
        if (payload.isEmpty()) throw FrameException(FrameException.Kind.EMPTY)
        if (payload.size > limit) throw FrameException(FrameException.Kind.TOO_LARGE)
        val frame = ByteArray(4 + payload.size)
        for (i in 0 until 4) frame[i] = (payload.size ushr (24 - 8 * i)).toByte()
        payload.copyInto(frame, 4)
        output.write(frame)
        output.flush()
    }

    private fun readFully(input: InputStream, buffer: ByteArray): Int {
        var offset = 0
        while (offset < buffer.size) {
            val n = input.read(buffer, offset, buffer.size - offset)
            if (n < 0) break
            offset += n
        }
        return offset
    }
}
