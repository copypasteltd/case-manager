package hk.copypaste.workspace;

import java.net.URI;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;

/** Short-lived provider capability. Never persisted or included in errors. */
final class UploadPermit {
    final String url;
    final Map<String, String> headers;
    UploadPermit(String url, Map<String, String> headers) { this.url = url; this.headers = Collections.unmodifiableMap(new LinkedHashMap<>(headers)); }
    static UploadPermit validate(String url, String method, int number, int expected, long expires, long now, Map<String, String> input) {
        if (url == null || url.length() > 8192 || input == null || input.size() > 32 || !"PUT".equals(method) || number != expected || expires < now + 10_000 || expires > now + 330_000) throw new IllegalArgumentException();
        URI uri = URI.create(url); String host = uri.getHost();
        if (!"https".equals(uri.getScheme()) || host == null || !(host.endsWith(".aliyuncs.com") || host.endsWith(".r2.cloudflarestorage.com")) || uri.getRawUserInfo() != null || uri.getRawFragment() != null || !(uri.getPort() == -1 || uri.getPort() == 443) || uri.getRawPath().isEmpty()) throw new IllegalArgumentException();
        Map<String, String> headers = new LinkedHashMap<>(); int bytes = 0;
        for (Map.Entry<String, String> entry : input.entrySet()) {
            String key = entry.getKey().toLowerCase(Locale.ROOT), value = entry.getValue();
            if (!key.matches("[a-z0-9-]+") || !(key.equals("content-type") || key.equals("content-md5") || key.startsWith("x-amz-") || key.startsWith("x-oss-")) || value == null || value.length() > 8192 || headers.containsKey(key)) throw new IllegalArgumentException();
            for (int index = 0; index < value.length(); index++) if (value.charAt(index) < 32 || value.charAt(index) == 127) throw new IllegalArgumentException();
            bytes += key.length() + value.length(); if (bytes > 16 * 1024) throw new IllegalArgumentException();
            headers.put(key, value);
        }
        return new UploadPermit(url, headers);
    }
}
