package io.nekohasekai.sagernet.ui.profile

import androidx.compose.runtime.Composable
import io.nekohasekai.sagernet.fmt.yggdrasil.YggdrasilBean
import io.nekohasekai.sagernet.ktx.applyDefaultValues
import io.nekohasekai.sagernet.ui.compose.YggdrasilProfileSettingsScreen
import moe.matsuri.nb4a.proxy.PreferenceBinding
import moe.matsuri.nb4a.proxy.PreferenceBindingManager
import moe.matsuri.nb4a.proxy.Type

class YggdrasilSettingsActivity : ProfileSettingsActivity<YggdrasilBean>() {
    override val usesComposePreferences = true

    override fun createEntity() = YggdrasilBean().applyDefaultValues()

    private val binding = PreferenceBindingManager().apply {
        add(PreferenceBinding(Type.Text, "name"))
        add(PreferenceBinding(Type.Text, "peers"))
        add(PreferenceBinding(Type.Text, "privateKey"))
        add(PreferenceBinding(Type.Text, "allowedPublicKeys"))
        add(PreferenceBinding(Type.Text, "groupPassword"))
        add(PreferenceBinding(Type.TextToInt, "mtu"))
    }

    override fun YggdrasilBean.init() = binding.writeToCacheAll(this)

    override fun YggdrasilBean.serialize() = binding.fromCacheAll(this)

    @Composable
    override fun ComposePreferences() = YggdrasilProfileSettingsScreen()
}
