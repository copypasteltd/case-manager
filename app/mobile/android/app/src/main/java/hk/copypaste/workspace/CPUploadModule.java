package hk.copypaste.workspace;

import android.app.Activity;
import android.content.ContentValues;
import android.content.Intent;
import android.net.Uri;
import android.provider.DocumentsContract;
import com.facebook.react.bridge.ActivityEventListener;
import com.facebook.react.bridge.Arguments;
import com.facebook.react.bridge.LifecycleEventListener;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.UiThreadUtil;
import com.facebook.react.bridge.WritableMap;
import com.facebook.react.modules.core.DeviceEventManagerModule;
import java.util.LinkedHashMap;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

/** React Native bridge carries only control metadata and receipts, never bytes. */
final class CPUploadModule extends ReactContextBaseJavaModule implements ActivityEventListener, LifecycleEventListener {
    private static final int PICK = 48_701;
    private final UploadManager manager;
    private volatile boolean foreground;
    private Promise selecting;
    private String replacing;
    private long selectGeneration;
    private volatile boolean disposed;
    private final java.util.concurrent.atomic.AtomicBoolean transferring = new java.util.concurrent.atomic.AtomicBoolean();
    CPUploadModule(ReactApplicationContext context) {
        super(context); manager = UploadManager.get(context);
        context.addActivityEventListener(this); context.addLifecycleEventListener(this);
        manager.events = (handle, run, state, sent, confirmed, size) -> {
            if (disposed || !context.hasActiveReactInstance()) return;
            WritableMap value = Arguments.createMap(); value.putString("handle", handle); value.putString("run", run); value.putString("state", state);
            value.putString("sent_bytes", Long.toString(sent)); value.putString("confirmed_bytes", Long.toString(confirmed)); value.putString("size_bytes", Long.toString(size));
            context.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter.class).emit("CPUploadStatus", value);
        };
    }
    @Override public String getName() { return "CPUpload"; }
    @ReactMethod public void addListener(String name) { }
    @ReactMethod public void removeListeners(double count) { }
    private interface Work { Object run(long generation, String owner) throws Exception; }
    private void work(Promise promise, Work action) {
        work(promise, action, false);
    }
    private void work(Promise promise, Work action, boolean part) {
        if (part && !transferring.compareAndSet(false, true)) { promise.reject("UPLOAD_BUSY", "当前已有分片正在传输。"); return; }
        long generation = manager.generation();
        try { (part ? manager.partIO : manager.io).execute(() -> {
            try {
                String owner = manager.owner(generation); Object result = action.run(generation, owner);
                manager.owner(generation); if (disposed) throw new IllegalStateException(); promise.resolve(result);
            } catch (Exception ignored) { promise.reject("UPLOAD_ACTION_UNCONFIRMED", "文件操作未能确认，请先刷新队列核对；文件权限变化时请重新选择原文件。"); }
            finally { if (part) transferring.set(false); }
        }); } catch (Exception ignored) { if (part) transferring.set(false); promise.reject("UPLOAD_BUSY", "当前文件操作较多，请稍后重试。"); }
    }
    @ReactMethod public void scope(String owner, Promise promise) {
        try { manager.scope(owner); promise.resolve(null); } catch (Exception ignored) { promise.reject("UPLOAD_SCOPE_CHANGED", "请重新登录并刷新队列。"); }
    }
    @ReactMethod public void clearScope() {
        try { manager.logout(); } catch (Exception ignored) { }
        UiThreadUtil.runOnUiThread(() -> { if (selecting != null) { selecting.reject("UPLOAD_CANCELLED", "文件选择已结束。"); selecting = null; replacing = null; } });
    }
    @ReactMethod public void list(Promise promise) {
        work(promise, (generation, owner) -> {
            JSONArray values = new JSONArray(); UploadManager.Run active = manager.active();
            for (UploadStore.Record record : manager.store.list(owner)) {
                if (record.state.equals("transferring") && (active == null || !active.handle.equals(record.id))) {
                    manager.store.state(owner, record.id, "paused"); record.state = "paused";
                }
                values.put(record.publicValue());
            }
            return values.toString();
        });
    }
    @ReactMethod public void select(String replacement, Promise promise) {
        UiThreadUtil.runOnUiThread(() -> {
            Activity activity = getCurrentActivity();
            try {
                if (activity == null || activity.isFinishing() || selecting != null || manager.active() != null) throw new IllegalStateException();
                selectGeneration = manager.generation(); String owner = manager.owner(selectGeneration);
                if (replacement != null && !replacement.isEmpty()) manager.store.get(owner, replacement);
                selecting = promise; replacing = replacement;
                Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT).setType("*/*").addCategory(Intent.CATEGORY_OPENABLE)
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION);
                activity.startActivityForResult(intent, PICK);
            } catch (Exception ignored) { selecting = null; replacing = null; promise.reject("UPLOAD_PICK_UNAVAILABLE", "请在前台选择一个可持久授权、支持随机读取的文件。"); }
        });
    }
    @Override public void onActivityResult(Activity activity, int requestCode, int resultCode, Intent data) {
        if (requestCode != PICK || selecting == null) return;
        Promise promise = selecting; String replacement = replacing; long generation = selectGeneration; selecting = null; replacing = null;
        if (resultCode != Activity.RESULT_OK || data == null || data.getData() == null) { promise.reject("UPLOAD_CANCELLED", "已取消文件选择。"); return; }
        Uri uri = data.getData(); int flags = data.getFlags();
        try { manager.io.execute(() -> {
            boolean stored = false;
            try {
                String owner = manager.owner(generation);
                if (!"content".equals(uri.getScheme()) || !DocumentsContract.isDocumentUri(getReactApplicationContext(), uri) ||
                    (flags & Intent.FLAG_GRANT_READ_URI_PERMISSION) == 0 || (flags & Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION) == 0) throw new IllegalArgumentException();
                getReactApplicationContext().getContentResolver().takePersistableUriPermission(uri, Intent.FLAG_GRANT_READ_URI_PERMISSION);
                SAFUploadSource source = new SAFUploadSource(getReactApplicationContext().getContentResolver(), uri);
                String fingerprint = UploadSource.fingerprint(source, source.name, source.size, source.modified); manager.owner(generation);
                UploadStore.Record record;
                if (replacement != null && !replacement.isEmpty()) {
                    record = manager.store.get(owner, replacement);
                    if (!fingerprint.equals(record.fingerprint) || source.size != record.size || !source.media.equals(record.media)) throw new IllegalArgumentException();
                    ContentValues values = new ContentValues(); values.put("uri", uri.toString()); values.put("state", "paused");
                    manager.store.update(owner, record.id, values); String previous = record.uri; record = manager.store.get(owner, record.id); manager.release(previous);
                } else record = manager.store.add(owner, uri.toString(), source, fingerprint);
                stored = true; manager.owner(generation); promise.resolve(record.publicValue().toString());
            } catch (Exception ignored) { promise.reject("UPLOAD_SOURCE_UNAVAILABLE", "文件未获持久读取授权、来源已变化或队列已满，请选择原文件并检查设备可用空间与权限。"); }
            finally { if (!stored) manager.release(uri.toString()); }
        }); } catch (Exception ignored) { promise.reject("UPLOAD_BUSY", "文件选择未保存，请稍后重试。"); }
    }
    @ReactMethod public void prepare(String handle, String input, Promise promise) {
        work(promise, (generation, owner) -> { if (input.length() > 4096) throw new IllegalArgumentException(); manager.prepare(generation, handle, new JSONObject(input)); return manager.store.get(owner, handle).publicValue().toString(); });
    }
    @ReactMethod public void bind(String handle, String input, Promise promise) {
        work(promise, (generation, owner) -> {
            if (input.length() > 256 * 1024) throw new IllegalArgumentException();
            JSONObject upload = new JSONObject(input); UploadStore.Record record = manager.store.get(owner, handle);
            String id = UploadStore.id(upload.getString("id")); long size = Long.parseLong(upload.getString("size_bytes"));
            UploadLayout layout = new UploadLayout(size, upload.getLong("part_bytes"), upload.getInt("part_count"));
            long epoch = upload.getLong("storage_epoch");
            if (size != record.size || !upload.getString("source_fingerprint").equals(record.fingerprint) || epoch < 1 ||
                (!record.upload.isEmpty() && (!record.upload.equals(id) || record.partBytes != layout.partBytes || record.partCount != layout.count || record.storageEpoch != epoch))) throw new IllegalArgumentException();
            if (!record.createBody.isEmpty()) {
                JSONObject original = new JSONObject(record.createBody);
                for (String key : new String[] {"folder_id", "name", "media_type"}) if (!original.getString(key).equals(upload.getString(key))) throw new IllegalArgumentException();
                for (String key : new String[] {"file_id", "base_version_id"}) {
                    String expected = original.isNull(key) ? "" : original.getString(key), actual = upload.isNull(key) ? "" : upload.getString(key);
                    if (!expected.equals(actual)) throw new IllegalArgumentException();
                }
            }
            ContentValues values = new ContentValues(); values.put("upload", id); values.put("part_bytes", layout.partBytes); values.put("part_count", layout.count); values.put("storage_epoch", epoch);
            manager.store.update(owner, handle, values); return manager.store.get(owner, handle).publicValue().toString();
        });
    }
    @ReactMethod public void begin(String handle, Promise promise) {
        if (!foreground) { promise.reject("UPLOAD_FOREGROUND_REQUIRED", "请回到应用前台再开始或继续上传。"); return; }
        work(promise, (generation, owner) -> { if (!foreground) throw new IllegalStateException(); return manager.begin(generation, handle).id; });
    }
    @ReactMethod public void touch(String handle, String run, String confirmed, String transferred, Promise promise) {
        try { manager.touch(handle, run, Long.parseLong(confirmed), Long.parseLong(transferred)); promise.resolve(null); }
        catch (Exception ignored) { promise.reject("UPLOAD_PAUSED", "本机上传已暂停，请刷新后继续。"); }
    }
    @ReactMethod public void transfer(String handle, String runID, double number, String input, Promise promise) {
        work(promise, (generation, owner) -> {
            UploadManager.Run run = manager.run(handle, runID); UploadStore.Record record = manager.store.get(owner, handle);
            if (record.upload.isEmpty() || number != (int) number || input.length() > 32 * 1024) throw new IllegalArgumentException();
            UploadLayout layout = new UploadLayout(record.size, record.partBytes, record.partCount); layout.start((int) number);
            JSONObject permit = new JSONObject(input); JSONArray array = permit.optJSONArray("headers"); Map<String, String> headers = new LinkedHashMap<>();
            if (array != null) for (int index = 0; index < array.length(); index++) {
                JSONObject header = array.getJSONObject(index); if (headers.put(header.getString("name"), header.getString("value")) != null) throw new IllegalArgumentException();
            }
            UploadPermit capability = UploadPermit.validate(permit.getString("url"), permit.getString("method"), permit.getInt("part_number"), (int) number, permit.getLong("expires_ms"), System.currentTimeMillis(), headers);
            run.busy = true;
            try (UploadTransfer transfer = new UploadTransfer()) {
                run.transfer = transfer; run.sourceCancellation = new android.os.CancellationSignal(); manager.run(handle, runID);
                String etag = transfer.put(manager.source(record, run.sourceCancellation), layout, (int) number, capability, sent -> manager.progress(run, sent));
                manager.run(handle, runID);
                return new JSONObject().put("part_number", (int) number).put("etag", etag).put("size_bytes", Long.toString(layout.length((int) number))).toString();
            } finally { run.transfer = null; run.sourceCancellation = null; run.busy = false; run.sent = 0; run.touched = android.os.SystemClock.elapsedRealtime(); }
        }, true);
    }
    @ReactMethod public void verifySource(String handle, Promise promise) {
        work(promise, (generation, owner) -> { UploadStore.Record record = manager.store.get(owner, handle); SAFUploadSource source = manager.source(record);
            if (!UploadSource.fingerprint(source, source.name, source.size, source.modified).equals(record.fingerprint)) throw new IllegalStateException(); return null; });
    }
    @ReactMethod public void finish(String handle, String run, String state, Promise promise) {
        try {
            if (!(state.equals("paused") || state.equals("uncertain") || state.equals("awaiting_verification") || state.equals("waiting_session"))) throw new IllegalArgumentException();
            manager.finish(handle, run, state); promise.resolve(null);
        } catch (Exception ignored) { promise.reject("UPLOAD_PAUSED", "本机上传已停止，请刷新队列核对。"); }
    }
    @ReactMethod public void pause() { manager.pause("paused"); }
    @ReactMethod public void forget(String handle, Promise promise) {
        work(promise, (generation, owner) -> {
            UploadManager.Run active = manager.active(); if (active != null && active.handle.equals(handle)) throw new IllegalStateException();
            UploadStore.Record record = manager.store.get(owner, handle); manager.store.delete(owner, handle); manager.release(record.uri); return null;
        });
    }
    @Override public void onNewIntent(Intent intent) { }
    @Override public void onHostResume() { foreground = true; }
    @Override public void onHostPause() { foreground = false; }
    @Override public void onHostDestroy() { foreground = false; }
    @Override public void invalidate() {
        disposed = true; manager.events = null; manager.pause("paused");
        getReactApplicationContext().removeActivityEventListener(this); getReactApplicationContext().removeLifecycleEventListener(this);
        super.invalidate();
    }
}
