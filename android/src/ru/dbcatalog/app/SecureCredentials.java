package ru.dbcatalog.app;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.util.Arrays;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import mobile.CredentialStore;

/** Implements the shared Go credential interface without a desktop keyring. */
final class SecureCredentials implements CredentialStore {
    private static final String ALIAS = "CatalogDB.Credentials.v1";
    private final SharedPreferences preferences;

    SecureCredentials(Context context) {
        preferences = context.getSharedPreferences("encrypted_credentials", Context.MODE_PRIVATE);
    }

    private SecretKey key() throws Exception {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore");
        store.load(null);
        if (!store.containsAlias(ALIAS)) {
            KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setRandomizedEncryptionRequired(true)
                    .setUserAuthenticationRequired(false)
                    .build());
            generator.generateKey();
        }
        return (SecretKey) store.getKey(ALIAS, null);
    }

    private byte[] binding(String service, String name) {
        return (service + "\u0000" + name).getBytes(StandardCharsets.UTF_8);
    }

    private String preferenceName(String service, String name) throws Exception {
        return Base64.encodeToString(MessageDigest.getInstance("SHA-256").digest(binding(service, name)),
                Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
    }

    @Override public synchronized String get(String service, String name) throws Exception {
        try {
            String value = preferences.getString(preferenceName(service, name), null);
            if (value == null) throw new Exception("catalogdb.secret.not-found");
            byte[] payload = Base64.decode(value, Base64.NO_WRAP);
            if (payload.length < 30 || payload[0] != 1) throw new Exception("Некорректное хранилище");
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.DECRYPT_MODE, key(), new GCMParameterSpec(128, Arrays.copyOfRange(payload, 1, 13)));
            cipher.updateAAD(binding(service, name));
            return new String(cipher.doFinal(payload, 13, payload.length - 13), StandardCharsets.UTF_8);
        } catch (Exception error) {
            if ("catalogdb.secret.not-found".equals(error.getMessage())) throw error;
            throw new Exception("Не удалось прочитать системное хранилище Android");
        }
    }

    @Override public synchronized void set(String service, String name, String value) throws Exception {
        try {
            String pref = preferenceName(service, name);
            if (value.isEmpty()) {
                if (!preferences.edit().remove(pref).commit()) throw new Exception("Не удалось очистить хранилище");
                return;
            }
            Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
            cipher.init(Cipher.ENCRYPT_MODE, key());
            cipher.updateAAD(binding(service, name));
            byte[] encrypted = cipher.doFinal(value.getBytes(StandardCharsets.UTF_8));
            byte[] iv = cipher.getIV();
            if (iv.length != 12) throw new Exception("Некорректный IV");
            byte[] payload = new byte[1 + iv.length + encrypted.length];
            payload[0] = 1;
            System.arraycopy(iv, 0, payload, 1, iv.length);
            System.arraycopy(encrypted, 0, payload, 13, encrypted.length);
            if (!preferences.edit().putString(pref, Base64.encodeToString(payload, Base64.NO_WRAP)).commit()) {
                throw new Exception("Не удалось записать хранилище");
            }
        } catch (Exception error) {
            throw new Exception("Не удалось записать системное хранилище Android");
        }
    }
}
