package hk.copypaste.workspace;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.net.URI;
import java.nio.ByteBuffer;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.Locale;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import okhttp3.Call;
import okhttp3.Callback;
import okhttp3.MediaType;
import okhttp3.OkHttpClient;
import okhttp3.Request;
import okhttp3.RequestBody;
import okhttp3.Response;
import okhttp3.ResponseBody;

/** Exact-origin JSON transport. It never accepts an arbitrary remote URL. */
final class ApiHTTP {
    static final int MAX_RESPONSE_BYTES = 2 * 1024 * 1024;
    static int responseLimit(String method,String path) {
        return method.equals("POST")&&path.matches("/v1/(files|folders)/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}/office-render") ? 18*1024*1024 : MAX_RESPONSE_BYTES;
    }
    static final int MAX_REQUEST_BYTES = 1024 * 1024;
    interface Result { void complete(int status, String body); void failed(); }
    private final URI origin;
    private final OkHttpClient client;
    private final ConcurrentHashMap<String, Call> active = new ConcurrentHashMap<>();
    private final Semaphore capacity = new Semaphore(16);
    private final AtomicBoolean closed = new AtomicBoolean();
    private final Object lifecycle = new Object();

    ApiHTTP(String value, boolean localHTTP) {
        origin = validateOrigin(value, localHTTP);
        client = new OkHttpClient.Builder().followRedirects(false).followSslRedirects(false)
            .retryOnConnectionFailure(false).callTimeout(20, TimeUnit.SECONDS)
            .connectTimeout(10, TimeUnit.SECONDS).readTimeout(15, TimeUnit.SECONDS).build();
    }
    static URI validateOrigin(String value, boolean localHTTP) {
        try {
            URI uri = new URI(value);
            boolean loopback = localHTTP && "http".equals(uri.getScheme()) && ("127.0.0.1".equals(uri.getHost()) || "localhost".equals(uri.getHost()));
            if ((!loopback && !"https".equals(uri.getScheme())) || uri.getHost() == null || uri.getRawUserInfo() != null || uri.getRawQuery() != null || uri.getRawFragment() != null || !(uri.getRawPath().isEmpty() || uri.getRawPath().equals("/")) || uri.getPort() == 0 || uri.getPort() > 65535) throw new IllegalArgumentException();
            return uri;
        } catch (Exception ignored) { throw new IllegalArgumentException("An explicit API origin is required."); }
    }
    URI target(String path) {
        if (path == null || path.length() > 4096 || !path.startsWith("/v1/") || path.contains("\\")) throw new IllegalArgumentException();
        URI relative = URI.create(path), uri = origin.resolve(relative);
        if (relative.getRawAuthority() != null || relative.getScheme() != null || relative.getRawFragment() != null || !uri.getScheme().equals(origin.getScheme()) || !uri.getRawAuthority().equals(origin.getRawAuthority()) || !uri.getRawPath().startsWith("/v1/")) throw new IllegalArgumentException();
        String decoded = relative.getPath();
        if (decoded.contains("\\")) throw new IllegalArgumentException();
        for (int index = 0; index < decoded.length(); index++) if (decoded.charAt(index) < 32 || decoded.charAt(index) == 127) throw new IllegalArgumentException();
        for (String segment : decoded.split("/")) if (segment.equals(".") || segment.equals("..")) throw new IllegalArgumentException();
        return uri;
    }
    void request(String id, String method, String path, Map<String, String> headers, String body, Result result) {
        boolean acquired = false;
        try {
            if (closed.get() || id == null || !id.matches("[1-9][0-9]{0,15}") || !(method.equals("GET") || method.equals("POST") || method.equals("PATCH")) || (method.equals("GET") && body != null)) throw new IllegalArgumentException();
            Request.Builder builder = new Request.Builder().url(target(path).toString()).header("Accept", "application/json");
            final int responseLimit=responseLimit(method,path);
            if (headers.size() > 3) throw new IllegalArgumentException();
            for (Map.Entry<String, String> header : headers.entrySet()) {
                String key = header.getKey().toLowerCase(Locale.ROOT), value = header.getValue();
                if (!(key.equals("authorization") || key.equals("idempotency-key") || key.equals("if-range")) || value == null || value.length() > 8192 || value.contains("\r") || value.contains("\n") || (key.equals("if-range") && !value.matches("\"[a-f0-9]{64}\""))) throw new IllegalArgumentException();
                builder.header(header.getKey(), value);
            }
            byte[] bytes = body == null ? new byte[0] : body.getBytes(StandardCharsets.UTF_8);
            if (bytes.length > MAX_REQUEST_BYTES) throw new IllegalArgumentException();
            builder.method(method, method.equals("GET") ? null : RequestBody.create(MediaType.get("application/json;charset=utf-8"), bytes));
            if (!capacity.tryAcquire()) throw new IllegalStateException();
            acquired = true;
            Call call = client.newCall(builder.build());
            if (active.putIfAbsent(id, call) != null) throw new IllegalStateException();
            acquired = false;
            AtomicBoolean released = new AtomicBoolean();
            Runnable release = () -> { if (released.compareAndSet(false, true)) { active.remove(id, call); capacity.release(); } };
            Callback callback = new Callback() {
                private void release() { release.run(); }
                @Override public void onFailure(Call failed, IOException error) { try { result.failed(); } finally { release(); } }
                @Override public void onResponse(Call completed, Response response) {
                    try (Response owned = response) {
                        ResponseBody source = owned.body();
                        if (owned.isRedirect() || source == null || source.contentLength() > responseLimit || source.contentType() == null || !source.contentType().type().equals("application") || !source.contentType().subtype().equals("json")) throw new IOException();
                        ByteArrayOutputStream buffer = new ByteArrayOutputStream();
                        byte[] chunk = new byte[8192]; int count;
                        while ((count = source.byteStream().read(chunk)) != -1) {
                            if (buffer.size() + count > responseLimit) throw new IOException();
                            buffer.write(chunk, 0, count);
                        }
                        Arrays.fill(chunk, (byte) 0);
                        byte[] raw = buffer.toByteArray();
                        try {
                            String value = StandardCharsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString();
                            result.complete(owned.code(), value);
                        } finally { Arrays.fill(raw, (byte) 0); }
                    } catch (Exception ignored) { result.failed(); }
                    finally { release(); }
                }
            };
            try {
                synchronized (lifecycle) {
                    if (closed.get()) throw new IllegalStateException();
                    call.enqueue(callback);
                }
            } catch (Exception ignored) { release.run(); result.failed(); }
        } catch (Exception ignored) { if (acquired) capacity.release(); result.failed(); }
    }
    void cancel(String id) { Call call = active.get(id); if (call != null) call.cancel(); }
    void close() { synchronized (lifecycle) { closed.set(true); for (Call call : active.values()) call.cancel(); client.connectionPool().evictAll(); client.dispatcher().executorService().shutdown(); } }
}
