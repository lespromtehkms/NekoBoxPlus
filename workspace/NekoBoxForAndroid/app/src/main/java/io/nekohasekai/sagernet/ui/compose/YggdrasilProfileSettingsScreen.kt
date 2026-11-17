package io.nekohasekai.sagernet.ui.compose

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import io.nekohasekai.sagernet.R
import io.nekohasekai.sagernet.database.DataStore

@Composable
internal fun YggdrasilProfileSettingsScreen() {
    val context = LocalContext.current
    val store = DataStore.profileCacheStore
    val notSet = stringResource(R.string.not_set)

    var name by remember { mutableStateOf(store.getString("name").orEmpty()) }
    var peers by remember { mutableStateOf(store.getString("peers").orEmpty()) }
    var privateKey by remember { mutableStateOf(store.getString("privateKey").orEmpty()) }
    var allowedPublicKeys by remember { mutableStateOf(store.getString("allowedPublicKeys").orEmpty()) }
    var groupPassword by remember { mutableStateOf(store.getString("groupPassword").orEmpty()) }
    var mtu by remember { mutableStateOf(store.getString("mtu").orEmpty().ifBlank { "65535" }) }

    fun summary(value: String): String {
        return value.ifBlank { notSet }
    }

    @Composable
    fun row(
        icon: Int,
        title: String,
        value: String,
        keyboard: KeyboardType = KeyboardType.Text,
        onUpdate: (String) -> Unit,
    ) {
        ListItem(
            modifier = Modifier
                .fillMaxWidth()
                .clickable {
                    context.showComposeTextInputDialog(
                        title = title,
                        initialValue = value,
                        keyboardType = keyboard,
                        maxLength = Int.MAX_VALUE,
                        password = false,
                        onPositive = onUpdate,
                    )
                },
            leadingContent = {
                Icon(painter = painterResource(id = icon), contentDescription = null)
            },
            headlineContent = {
                Text(text = title)
            },
            supportingContent = {
                Text(text = summary(value))
            },
        )
    }

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .navigationBarsPadding()
    ) {
        item {
            row(
                icon = R.drawable.ic_social_emoji_symbols,
                title = stringResource(R.string.profile_name),
                value = name,
            ) {
                name = it
                store.putString("name", it)
            }
        }
        item {
            row(
                icon = R.drawable.ic_baseline_domain_24,
                title = "Peers (http://, httptls://, tls://, tcp://)",
                value = peers,
            ) {
                peers = it
                store.putString("peers", it)
            }
        }
        item {
            row(
                icon = R.drawable.ic_settings_password,
                title = "PrivateKey (hex, пусто = автогенерация)",
                value = privateKey,
            ) {
                privateKey = it.trim()
                store.putString("privateKey", privateKey)
            }
        }
        item {
            row(
                icon = R.drawable.ic_baseline_fingerprint_24,
                title = "AllowedPublicKeys (Firewall)",
                value = allowedPublicKeys,
            ) {
                allowedPublicKeys = it
                store.putString("allowedPublicKeys", it)
            }
        }
        item {
            row(
                icon = R.drawable.ic_settings_password,
                title = "GroupPassword",
                value = groupPassword,
            ) {
                groupPassword = it
                store.putString("groupPassword", it)
            }
        }
        item {
            row(
                icon = R.drawable.ic_action_settings,
                title = "MTU",
                value = mtu,
                keyboard = KeyboardType.Number,
            ) {
                mtu = it.trim()
                store.putString("mtu", mtu)
            }
        }
        item {
            SharedDialOptions()
        }
    }
}
