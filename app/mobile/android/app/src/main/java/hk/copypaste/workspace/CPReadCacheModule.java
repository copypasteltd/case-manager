package hk.copypaste.workspace;

import android.app.Activity;
import android.content.ClipData;
import android.content.Intent;
import android.net.Uri;
import android.view.WindowManager;
import androidx.core.content.FileProvider;
import com.facebook.react.bridge.BaseActivityEventListener;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.UiThreadUtil;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

public final class CPReadCacheModule extends ReactContextBaseJavaModule {
    private static final int OPEN_DOCUMENT = 44201;
    private final ThreadPoolExecutor io = new ThreadPoolExecutor(1, 1, 0, TimeUnit.SECONDS, new ArrayBlockingQueue<>(8));
    private ReadCache cache;
    private final AtomicReference<Promise> opening = new AtomicReference<>();
    private Uri granted;
    CPReadCacheModule(ReactApplicationContext context) {
        super(context);
        context.addActivityEventListener(new BaseActivityEventListener() {
            @Override public void onActivityResult(Activity activity, int requestCode, int resultCode, Intent data) {
                if (requestCode != OPEN_DOCUMENT) return;
                revoke(); Promise promise = opening.getAndSet(null); if (promise != null) promise.resolve(null);
            }
        });
    }
    @Override public String getName() { return "CPReadCache"; }
    private interface Operation { Object run(ReadCache value) throws Exception; }
    private void perform(Promise promise, Operation operation) {
        try {
            io.execute(() -> {
                try {
                    if (cache == null) cache = new ReadCache(getReactApplicationContext().getCacheDir());
                    promise.resolve(operation.run(cache));
                } catch (Exception ignored) { promise.reject("CONTENT_CACHE_UNAVAILABLE", "Private temporary content is unavailable."); }
            });
        } catch (Exception ignored) { promise.reject("CONTENT_CACHE_UNAVAILABLE", "Private temporary content is unavailable."); }
    }
    @ReactMethod public void clear(Promise promise) { perform(promise, value -> { if (opening.get() != null) throw new IllegalStateException(); value.clear(); return null; }); }
    @ReactMethod public void create(String version, String extension, Promise promise) { perform(promise, value -> value.create(version, extension)); }
    @ReactMethod public void append(String handle, String base64, Promise promise) { perform(promise, value -> { value.append(handle, base64); return null; }); }
    @ReactMethod public void finish(String handle, double bytes, Promise promise) { perform(promise, value -> value.finish(handle, bytes)); }
    @ReactMethod public void remove(String handle, Promise promise) { perform(promise, value -> { value.remove(handle); return null; }); }
    @ReactMethod public void protectScreen() {
        UiThreadUtil.runOnUiThread(() -> { Activity activity = getCurrentActivity(); if (activity != null) activity.getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE); });
    }
    private void revoke() {
        if (granted != null) { getReactApplicationContext().revokeUriPermission(granted, Intent.FLAG_GRANT_READ_URI_PERMISSION); granted = null; }
    }
    @ReactMethod public void open(String uri, Promise promise) {
        if (!opening.compareAndSet(null, promise)) { promise.reject("CONTENT_VIEW_UNAVAILABLE", "A document is already open."); return; }
        try {
            io.execute(() -> {
                try {
                    if (cache == null) throw new IllegalStateException();
                    ReadCache.Entry value = cache.byURI(uri);
                    Uri content = FileProvider.getUriForFile(getReactApplicationContext(), getReactApplicationContext().getPackageName() + ".readcache", value.file);
                    String mime = ReadCache.mime(value.extension);
                    UiThreadUtil.runOnUiThread(() -> {
                        try {
                            Activity activity = getCurrentActivity(); if (activity == null) throw new IllegalStateException();
                            Intent intent = new Intent(Intent.ACTION_VIEW).setDataAndType(content, mime).addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
                            intent.setClipData(ClipData.newRawUri("Fixed version", content));
                            granted = content;
                            activity.startActivityForResult(intent, OPEN_DOCUMENT);
                        } catch (Exception ignored) { failedOpen(promise); }
                    });
                } catch (Exception ignored) { failedOpen(promise); }
            });
        } catch (Exception ignored) { failedOpen(promise); }
    }
    private void failedOpen(Promise promise) {
        UiThreadUtil.runOnUiThread(() -> { if (opening.compareAndSet(promise, null)) { revoke(); promise.reject("CONTENT_VIEW_UNAVAILABLE", "No document viewer is available."); } });
    }
    @Override public void invalidate() {
        UiThreadUtil.runOnUiThread(() -> { revoke(); Promise promise = opening.getAndSet(null); if (promise != null) promise.reject("CONTENT_VIEW_UNAVAILABLE", "The content session has ended."); });
        try { io.execute(() -> { try { if (cache != null) cache.clear(); } catch (Exception ignored) {} }); } catch (Exception ignored) {}
        io.shutdown();
        super.invalidate();
    }
}
