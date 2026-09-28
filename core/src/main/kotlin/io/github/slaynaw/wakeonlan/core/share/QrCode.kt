package io.github.slaynaw.wakeonlan.core.share

/**
 * QR code (mode octets, correction d'erreurs M, masque 0), pour afficher les liens de partage.
 * Même résultat que l'application Windows et l'agent (rsc.io/qr), vérifié par les tests.
 */
class QrCode private constructor(val size: Int, private val modules: Array<BooleanArray>) {

    /** Module noir en colonne [x], ligne [y]. */
    fun isDark(x: Int, y: Int): Boolean = modules[y][x]

    companion object {
        private const val MAX_VERSION = 40

        // Correction M : octets de correction par bloc et nombre de blocs, versions 1 à 40.
        private val ECC_PER_BLOCK = intArrayOf(
            10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26,
            26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28,
        )
        private val NUM_BLOCKS = intArrayOf(
            1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16,
            17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49,
        )

        /** Encode [text] (UTF-8) dans la plus petite version possible. */
        fun encode(text: String): QrCode {
            val data = text.toByteArray(Charsets.UTF_8)
            val version = (1..MAX_VERSION).firstOrNull { v ->
                4 + countBits(v) + data.size * 8 <= dataCodewords(v) * 8
            } ?: throw IllegalArgumentException("texte trop long pour un QR code")
            val bits = BitBuffer()
            bits.append(0b0100, 4)
            bits.append(data.size, countBits(version))
            data.forEach { bits.append(it.toInt() and 0xFF, 8) }
            val capacity = dataCodewords(version) * 8
            bits.append(0, minOf(4, capacity - bits.length))
            bits.append(0, (8 - bits.length % 8) % 8)
            var pad = 0xEC
            while (bits.length < capacity) {
                bits.append(pad, 8)
                pad = pad xor 0xEC xor 0x11
            }
            return Builder(version).build(interleave(bits.toBytes(), version))
        }

        private fun countBits(version: Int) = if (version <= 9) 8 else 16

        private fun rawDataModules(version: Int): Int {
            var result = (16 * version + 128) * version + 64
            if (version >= 2) {
                val align = version / 7 + 2
                result -= (25 * align - 10) * align - 55
                if (version >= 7) result -= 36
            }
            return result
        }

        private fun dataCodewords(version: Int): Int =
            rawDataModules(version) / 8 - ECC_PER_BLOCK[version - 1] * NUM_BLOCKS[version - 1]

        /** Découpe en blocs, ajoute la correction Reed-Solomon et entrelace. */
        private fun interleave(data: ByteArray, version: Int): ByteArray {
            val numBlocks = NUM_BLOCKS[version - 1]
            val eccLen = ECC_PER_BLOCK[version - 1]
            val raw = rawDataModules(version) / 8
            val numShort = numBlocks - raw % numBlocks
            val shortLen = raw / numBlocks
            val divisor = rsDivisor(eccLen)
            val blocks = ArrayList<ByteArray>()
            var k = 0
            for (i in 0 until numBlocks) {
                val len = shortLen - eccLen + if (i < numShort) 0 else 1
                val dat = data.copyOfRange(k, k + len)
                k += len
                val ecc = rsRemainder(dat, divisor)
                blocks += if (i < numShort) dat + byteArrayOf(0) + ecc else dat + ecc
            }
            val out = ByteArray(raw)
            var n = 0
            for (i in blocks[0].indices) {
                for (j in blocks.indices) {
                    if (i != shortLen - eccLen || j >= numShort) out[n++] = blocks[j][i]
                }
            }
            return out
        }

        private fun rsDivisor(degree: Int): IntArray {
            val result = IntArray(degree)
            result[degree - 1] = 1
            var root = 1
            repeat(degree) {
                for (j in result.indices) {
                    result[j] = gfMultiply(result[j], root)
                    if (j + 1 < result.size) result[j] = result[j] xor result[j + 1]
                }
                root = gfMultiply(root, 0x02)
            }
            return result
        }

        private fun rsRemainder(data: ByteArray, divisor: IntArray): ByteArray {
            val result = IntArray(divisor.size)
            for (b in data) {
                val factor = (b.toInt() and 0xFF) xor result[0]
                System.arraycopy(result, 1, result, 0, result.size - 1)
                result[result.size - 1] = 0
                for (i in result.indices) result[i] = result[i] xor gfMultiply(divisor[i], factor)
            }
            return ByteArray(result.size) { result[it].toByte() }
        }

        private fun gfMultiply(x: Int, y: Int): Int {
            var z = 0
            for (i in 7 downTo 0) {
                z = (z shl 1) xor ((z ushr 7) * 0x11D)
                z = z xor (((y ushr i) and 1) * x)
            }
            return z
        }
    }

    private class BitBuffer {
        private val bits = ArrayList<Boolean>()
        val length: Int get() = bits.size

        fun append(value: Int, count: Int) {
            for (i in count - 1 downTo 0) bits += (value ushr i) and 1 != 0
        }

        fun toBytes(): ByteArray = ByteArray(bits.size / 8) { i ->
            var b = 0
            for (j in 0 until 8) b = (b shl 1) or if (bits[i * 8 + j]) 1 else 0
            b.toByte()
        }
    }

    /** Placement des motifs fixes et des données (norme ISO/IEC 18004). */
    private class Builder(private val version: Int) {
        private val size = version * 4 + 17
        private val modules = Array(size) { BooleanArray(size) }
        private val function = Array(size) { BooleanArray(size) }

        fun build(codewords: ByteArray): QrCode {
            drawFunctionPatterns()
            drawCodewords(codewords)
            applyMask0()
            drawFormatBits()
            return QrCode(size, modules)
        }

        private fun set(x: Int, y: Int, dark: Boolean) {
            modules[y][x] = dark
            function[y][x] = true
        }

        private fun drawFunctionPatterns() {
            for (i in 0 until size) {
                set(6, i, i % 2 == 0)
                set(i, 6, i % 2 == 0)
            }
            drawFinder(3, 3)
            drawFinder(size - 4, 3)
            drawFinder(3, size - 4)
            val positions = alignmentPositions()
            val n = positions.size
            for (i in 0 until n) {
                for (j in 0 until n) {
                    if ((i == 0 && j == 0) || (i == 0 && j == n - 1) || (i == n - 1 && j == 0)) continue
                    for (dy in -2..2) for (dx in -2..2) set(positions[i] + dx, positions[j] + dy, maxOf(Math.abs(dx), Math.abs(dy)) != 1)
                }
            }
            drawFormatBits()
            drawVersion()
        }

        private fun drawFinder(x: Int, y: Int) {
            for (dy in -4..4) {
                for (dx in -4..4) {
                    val dist = maxOf(Math.abs(dx), Math.abs(dy))
                    val xx = x + dx
                    val yy = y + dy
                    if (xx in 0 until size && yy in 0 until size) set(xx, yy, dist != 2 && dist != 4)
                }
            }
        }

        private fun alignmentPositions(): IntArray {
            if (version == 1) return IntArray(0)
            val n = version / 7 + 2
            val step = if (version == 32) 26 else (version * 4 + n * 2 + 1) / (n * 2 - 2) * 2
            val result = IntArray(n)
            result[0] = 6
            var pos = size - 7
            for (i in n - 1 downTo 1) {
                result[i] = pos
                pos -= step
            }
            return result
        }

        /** Informations de format : correction M (00), masque 0. */
        private fun drawFormatBits() {
            val data = 0
            var rem = data
            repeat(10) { rem = (rem shl 1) xor ((rem ushr 9) * 0x537) }
            val bits = ((data shl 10) or rem) xor 0x5412
            fun bit(i: Int) = (bits ushr i) and 1 != 0
            for (i in 0..5) set(8, i, bit(i))
            set(8, 7, bit(6))
            set(8, 8, bit(7))
            set(7, 8, bit(8))
            for (i in 9 until 15) set(14 - i, 8, bit(i))
            for (i in 0 until 8) set(size - 1 - i, 8, bit(i))
            for (i in 8 until 15) set(8, size - 15 + i, bit(i))
            set(8, size - 8, true)
        }

        private fun drawVersion() {
            if (version < 7) return
            var rem = version
            repeat(12) { rem = (rem shl 1) xor ((rem ushr 11) * 0x1F25) }
            val bits = (version shl 12) or rem
            for (i in 0 until 18) {
                val bit = (bits ushr i) and 1 != 0
                val a = size - 11 + i % 3
                val b = i / 3
                set(a, b, bit)
                set(b, a, bit)
            }
        }

        private fun drawCodewords(data: ByteArray) {
            var i = 0
            var right = size - 1
            while (right >= 1) {
                if (right == 6) right = 5
                for (vert in 0 until size) {
                    for (j in 0..1) {
                        val x = right - j
                        val upward = (right + 1) and 2 == 0
                        val y = if (upward) size - 1 - vert else vert
                        if (!function[y][x] && i < data.size * 8) {
                            modules[y][x] = (data[i ushr 3].toInt() ushr (7 - (i and 7))) and 1 != 0
                            i++
                        }
                    }
                }
                right -= 2
            }
        }

        private fun applyMask0() {
            for (y in 0 until size) {
                for (x in 0 until size) {
                    if (!function[y][x] && (x + y) % 2 == 0) modules[y][x] = !modules[y][x]
                }
            }
        }
    }
}
