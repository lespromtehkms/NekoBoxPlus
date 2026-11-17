package io.nekohasekai.sagernet.fmt.yggdrasil

import moe.matsuri.nb4a.SingBoxOptions
import java.security.SecureRandom

private fun parseCleanList(raw: String?): List<String> {
    if (raw.isNullOrBlank()) return emptyList()
    return raw.lineSequence()
        .flatMap { it.splitToSequence(',') }
        .map { it.trim().trim('"', '\'') }
        .filter { it.isNotBlank() && !it.startsWith("#") }
        .toList()
}

fun buildSingBoxEndpointYggdrasilBean(
    bean: YggdrasilBean,
): SingBoxOptions.YggdrasilEndpointOptions = SingBoxOptions.YggdrasilEndpointOptions().apply {
    type = "yggdrasil"
    if (bean.privateKey.isNullOrBlank()) {
        val seed = ByteArray(32)
        SecureRandom().nextBytes(seed)
        bean.privateKey = seed.joinToString("") { "%02x".format(it) }
    }
    private_key = bean.privateKey.trim().trim('"', '\'')
    peers = parseCleanList(bean.peers).takeIf { it.isNotEmpty() }
    allowed_public_keys = parseCleanList(bean.allowedPublicKeys).takeIf { it.isNotEmpty() }
    group_password = bean.groupPassword?.takeIf { it.isNotEmpty() }
    mtu = bean.mtu?.takeIf { it in 1280..65535 } ?: 65535
}
