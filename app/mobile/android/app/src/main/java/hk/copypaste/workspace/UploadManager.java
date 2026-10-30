package hk.copypaste.workspace;

import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import androidx.core.content.ContextCompat;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;
import org.json.JSONObject;

/** The service owns transfer lifetime; JS retains the existing authenticated API protocol. */
final class UploadManager {
    interface Events { void changed(String handle, String run, String state, long sent, long confirmed, long size); }
    static final class Run {
        final String id = UUID.randomUUID().toString(); final String owner, handle; final long size;
        volatile long touched = SystemClock.elapsedRealtime(), sent, confirmed, transferred, emitted;
        volatile boolean busy;
        volatile UploadTransfer transfer;
        volatile android.os.CancellationSignal sourceCancellation;
        Run(String owner, UploadStore.Record record) { this.owner = owner; handle = record.id; size = record.size; }
    }
    private static UploadManager instance;
    static synchronized UploadManager get(Context context) { if (instance == null) instance = new UploadManager(context.getApplicationContext()); return instance; }
    final Context context;
    final UploadStore store;
    final ThreadPoolExecutor io = new ThreadPoolExecutor(1, 1, 30, TimeUnit.SECONDS, new ArrayBlockingQueue<>(16));
    final ThreadPoolExecutor partIO = new ThreadPoolExecutor(1, 1, 30, TimeUnit.SECONDS, new ArrayBlockingQueue<>(1));
    final Handler main = new Handler(Looper.getMainLooper());
    private String owner;
    private long generation;
    private Run active;
    volatile Events events;
    volatile UploadServiceHost host;
    private UploadManager(Context context) { this.context = context; store = new UploadStore(context); }
    synchronized long scope(String value) {
        UploadStore.id(value);
        if (!value.equals(owner)) { pause("paused"); owner = value; generation++; }
        return generation;
    }
    synchronized long generation() { return generation; }
    synchronized String owner(long expected) { if (owner == null || expected != generation) throw new IllegalStateException(); return owner; }
    synchronized void logout() {
        String previous = owner; owner = null; generation++; pause("paused");
        if (previous != null) io.execute(() -> {
            for (UploadStore.Record record : store.list(previous)) {
                ContentValues values = new ContentValues(); values.put("uri", ""); values.put("state", "needs_source");
                store.update(previous, record.id, values); release(record.uri);
            }
        });
    }
    void release(String uri) {
        if (uri == null || uri.isEmpty() || store.hasURI(uri)) return;
        if(uri.startsWith("cp-generated:")){try{GeneratedUploadSource.resolve(context,uri).delete();}catch(Exception ignored){}return;}
        try { context.getContentResolver().releasePersistableUriPermission(Uri.parse(uri), Intent.FLAG_GRANT_READ_URI_PERMISSION); } catch (Exception ignored) { }
    }
    SAFUploadSource source(UploadStore.Record record) throws Exception {
        return source(record, new android.os.CancellationSignal());
    }
    String fingerprint(SAFUploadSource source, String expected) throws Exception {
        return expected.startsWith("office-artifact-v1:") ? UploadSource.officeFingerprint(source,source.size) : UploadSource.fingerprint(source,source.name,source.size,source.modified);
    }
    SAFUploadSource source(UploadStore.Record record, android.os.CancellationSignal cancellation) throws Exception {
        if (record.uri.isEmpty()) throw new IllegalStateException();
        SAFUploadSource source = record.uri.startsWith("cp-generated:") ? GeneratedUploadSource.open(context,record,cancellation) : new SAFUploadSource(context.getContentResolver(), Uri.parse(record.uri), cancellation);
        if (!source.name.equals(record.name) || source.size != record.size || source.modified != record.modified || !source.media.equals(record.media)) throw new IllegalStateException();
        return source;
    }
    Run begin(long expected, String handle) throws Exception {
        String current;
        synchronized (this) { if (active != null) throw new IllegalStateException(); current = owner(expected); }
        UploadStore.Record record = store.get(current, handle);
        SAFUploadSource source = source(record);
        if (!fingerprint(source,record.fingerprint).equals(record.fingerprint)) throw new IllegalStateException();
        synchronized (this) {
            owner(expected); if (active != null) throw new IllegalStateException();
            Run run = new Run(current, record); active = run;
            try {
                store.state(current, handle, "transferring");
                ContextCompat.startForegroundService(context, new Intent(context, UploadServiceHost.class).putExtra("run", run.id));
                emit(run, "transferring"); return run;
            } catch (Exception failure) { active = null; store.state(current, handle, "paused"); throw failure; }
        }
    }
    synchronized Run run(String handle, String id) {
        Run run = active;
        if (run == null || !run.id.equals(id) || !run.handle.equals(handle) || !run.owner.equals(owner)) throw new IllegalStateException();
        return run;
    }
    synchronized Run active() { return active; }
    synchronized void touch(String handle, String id, long confirmed, long transferred) {
        Run run = run(handle, id); if (confirmed < 0 || confirmed > run.size || transferred < 0 || transferred > run.size) throw new IllegalArgumentException();
        run.confirmed = confirmed; run.transferred = transferred; run.sent = 0; run.touched = SystemClock.elapsedRealtime(); emit(run, "transferring");
    }
    synchronized void finish(String handle, String id, String state) {
        Run run = run(handle, id); pause(state);
    }
    synchronized void pause(String state) {
        Run run = active; if (run == null) return; active = null;
        UploadTransfer transfer = run.transfer; if (transfer != null) transfer.cancel();
        android.os.CancellationSignal cancellation = run.sourceCancellation; if (cancellation != null) cancellation.cancel();
        // Metadata writes are serialized behind any finishing native part.
        try { io.execute(() -> store.state(run.owner, run.handle, state)); } catch (Exception ignored) { }
        emit(run, state); UploadServiceHost service = host; if (service != null) main.post(() -> service.stopRun(run.id));
    }
    synchronized void progress(Run run, long sent) {
        if (active != run) return;
        run.sent = sent; run.touched = SystemClock.elapsedRealtime();
        if (run.touched - run.emitted >= 250) { run.emitted = run.touched; emit(run, "transferring"); }
    }
    synchronized void emit(Run run, String state) {
        Events listener = events; if (listener != null) listener.changed(run.handle, run.id, state, run.sent, run.confirmed, run.size);
    }
    synchronized void watchdog(String runID) {
        if (active != null && active.id.equals(runID) && !active.busy && SystemClock.elapsedRealtime() - active.touched > 60_000) pause("paused");
    }
    synchronized void prepare(long expected, String handle, JSONObject input) throws Exception {
        String current = owner(expected); UploadStore.Record record = store.get(current, handle);
        if (!record.upload.isEmpty() || !record.createBody.isEmpty() || active != null && active.handle.equals(handle)) throw new IllegalStateException();
        UploadStore.id(input.getString("folder_id")); String name = input.getString("name");
        if (name.isEmpty() || name.length() > 255 || name.contains("/") || name.contains("\\") || name.equals(".") || name.equals("..")) throw new IllegalArgumentException();
        for (int index = 0; index < name.length(); index++) if (name.charAt(index) < 32 || name.charAt(index) == 127) throw new IllegalArgumentException();
        boolean file = input.has("file_id"), base = input.has("base_version_id"); if (file != base) throw new IllegalArgumentException();
        JSONObject body = new JSONObject().put("folder_id", input.getString("folder_id")).put("name", name).put("media_type", record.media)
            .put("size_bytes", Long.toString(record.size)).put("source_fingerprint", record.fingerprint);
        if (file) body.put("file_id", UploadStore.id(input.getString("file_id"))).put("base_version_id", UploadStore.id(input.getString("base_version_id")));
        ContentValues values = new ContentValues(); values.put("create_body", body.toString()); values.put("prepared_at", System.currentTimeMillis()); values.put("state", "prepared"); store.update(current, handle, values);
    }
}
