package io.nekohasekai.sagernet.fmt.yggdrasil;

import androidx.annotation.NonNull;

import com.esotericsoftware.kryo.io.ByteBufferInput;
import com.esotericsoftware.kryo.io.ByteBufferOutput;

import org.jetbrains.annotations.NotNull;

import io.nekohasekai.sagernet.fmt.AbstractBean;
import io.nekohasekai.sagernet.fmt.KryoConverters;

public class YggdrasilBean extends AbstractBean {

    public String privateKey;
    public String peers;
    public String allowedPublicKeys;
    public String groupPassword;
    public Integer mtu;

    @Override
    public void initializeDefaultValues() {
        super.initializeDefaultValues();
        if (privateKey == null) privateKey = "";
        if (peers == null) peers = "";
        if (allowedPublicKeys == null) allowedPublicKeys = "";
        if (groupPassword == null) groupPassword = "";
        if (mtu == null || mtu <= 0) mtu = 65535;
        if (name == null || name.isBlank()) name = "Yggdrasil";
    }

    @Override
    public void serialize(ByteBufferOutput output) {
        output.writeInt(1);
        super.serialize(output);
        output.writeString(privateKey);
        output.writeString(peers);
        output.writeString(allowedPublicKeys);
        output.writeString(groupPassword);
        output.writeInt(mtu);
    }

    @Override
    public void deserialize(ByteBufferInput input) {
        input.readInt();
        super.deserialize(input);
        privateKey = input.readString();
        peers = input.readString();
        allowedPublicKeys = input.readString();
        groupPassword = input.readString();
        mtu = input.readInt();
    }

    @Override
    public boolean canTCPing() {
        return false;
    }

    @NotNull
    @Override
    public String displayAddress() {
        if (peers != null && !peers.isBlank()) {
            String first = peers.trim().split("[\\r\\n,\\s]+")[0];
            first = first.replaceAll("^[\"']+|[\"']+$", "");
            if (!first.isBlank()) return first;
        }
        return "Yggdrasil Mesh";
    }

    @NotNull
    @Override
    public String getHash() {
        return buildTypedHash("yggdrasil");
    }

    @NotNull
    @Override
    public YggdrasilBean clone() {
        return KryoConverters.deserialize(new YggdrasilBean(), KryoConverters.serialize(this));
    }

    public static final Creator<YggdrasilBean> CREATOR = new CREATOR<YggdrasilBean>() {
        @NonNull
        @Override
        public YggdrasilBean newInstance() {
            return new YggdrasilBean();
        }

        @Override
        public YggdrasilBean[] newArray(int size) {
            return new YggdrasilBean[size];
        }
    };
}
