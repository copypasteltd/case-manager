package hk.copypaste.workspace;

import java.io.IOException;
import java.io.InputStream;
import java.util.Arrays;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import okhttp3.Call;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okio.BufferedSink;

/** One part, one streaming buffer, no redirects, cookies, bearer, or retries. */
final class UploadTransfer implements AutoCloseable {
    interface Progress { void bytes(long count); }
    private final OkHttpClient client = new OkHttpClient.Builder().followRedirects(false).followSslRedirects(false)
        .retryOnConnectionFailure(false).connectTimeout(15, TimeUnit.SECONDS).readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS).callTimeout(10, TimeUnit.MINUTES).build();
    private final AtomicBoolean cancelled = new AtomicBoolean();
    private volatile Call active;
    private volatile InputStream input;
    String put(UploadSource source, UploadLayout layout, int number, UploadPermit permit, Progress progress) throws IOException {
        long start = layout.start(number), length = layout.length(number); source.check();
        RequestBody body = new RequestBody() {
            @Override public MediaType contentType() { return null; }
            @Override public long contentLength() { return length; }
            @Override public boolean isOneShot() { return true; }
            @Override public void writeTo(BufferedSink sink) throws IOException {
                byte[] buffer = new byte[64 * 1024];
                try (InputStream owned = source.open(start, length)) {
                    input = owned; long remaining = length, sent = 0;
                    while (remaining > 0) {
                        if (cancelled.get()) throw new IOException();
                        int count = owned.read(buffer, 0, (int) Math.min(buffer.length, remaining));
                        if (count <= 0) throw new IOException();
                        sink.write(buffer, 0, count); remaining -= count; sent += count; progress.bytes(sent);
                    }
                } finally { input = null; Arrays.fill(buffer, (byte) 0); }
            }
        };
        Request.Builder builder = new Request.Builder().url(permit.url).put(body);
        for (Map.Entry<String, String> header : permit.headers.entrySet()) builder.header(header.getKey(), header.getValue());
        Call call = client.newCall(builder.build()); active = call;
        if (cancelled.get()) call.cancel();
        try (Response response = call.execute()) {
            if (cancelled.get() || response.code() != 200 || response.isRedirect()) throw new IOException();
            String etag = response.header("ETag");
            if (etag == null || etag.isEmpty() || etag.length() > 200) throw new IOException();
            for (int index = 0; index < etag.length(); index++) if (etag.charAt(index) < 32 || etag.charAt(index) == 127) throw new IOException();
            source.check(); return etag;
        } catch (Exception ignored) { throw new IOException("Part result unconfirmed; reconcile before continuing."); }
        finally { active = null; }
    }
    void cancel() {
        cancelled.set(true); Call call = active; if (call != null) call.cancel();
        InputStream source = input; if (source != null) try { source.close(); } catch (IOException ignored) { }
    }
    @Override public void close() { cancel(); client.connectionPool().evictAll(); client.dispatcher().executorService().shutdown(); }
}
