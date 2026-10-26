package hk.copypaste.workspace;

import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Context;
import android.os.Build;
import android.os.PersistableBundle;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;

/** Explicit user-initiated proof copying; no clipboard read or background access. */
public final class CPClipboardModule extends ReactContextBaseJavaModule {
    CPClipboardModule(ReactApplicationContext context) { super(context); }
    @Override public String getName() { return "CPClipboard"; }

    @ReactMethod public void copyTelegramProof(String value, Promise promise) {
        if (value == null || value.length() > 180 || !value.matches(
                "(?:https://t\\.me/[A-Za-z0-9_]{5,32}\\?start=|/start )[A-Za-z0-9_-]{43}")) {
            promise.reject("invalid_proof", "The selected Telegram proof is invalid.");
            return;
        }
        getReactApplicationContext().runOnUiQueueThread(() -> {
            try {
                if (getCurrentActivity() == null) throw new IllegalStateException();
                ClipboardManager clipboard = (ClipboardManager) getReactApplicationContext()
                        .getSystemService(Context.CLIPBOARD_SERVICE);
                if (clipboard == null) throw new IllegalStateException();
                ClipData clip = ClipData.newPlainText("Telegram private proof", value);
                if (Build.VERSION.SDK_INT >= 24) {
                    PersistableBundle extras = new PersistableBundle();
                    extras.putBoolean("android.content.extra.IS_SENSITIVE", true);
                    clip.getDescription().setExtras(extras);
                }
                clipboard.setPrimaryClip(clip);
                promise.resolve(null);
            } catch (RuntimeException error) {
                promise.reject("copy_unavailable", "Clipboard is unavailable in the current foreground activity.");
            }
        });
    }
}
