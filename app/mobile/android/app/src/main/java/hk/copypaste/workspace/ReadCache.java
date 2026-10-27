package hk.copypaste.workspace;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;
import java.util.UUID;
import okio.ByteString;

/** Private, finite scratch storage. No user-provided path is ever opened. */
final class ReadCache {
    static final int MAX_FILE_BYTES = 32 * 1024 * 1024;
    static final int MAX_CHUNK_BYTES = 65536;
    static final String ID = "[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}";
    private static final String EXT = "png|jpg|gif|webp|pdf|csv|doc|docx|xls|xlsx|ppt|pptx";
    private final File root;
    private final Map<String, Entry> entries = new HashMap<>();
    static final class Entry {
        final File file; final String extension; long bytes; boolean finished;
        Entry(File file, String extension) { this.file = file; this.extension = extension; }
        String uri() { return "file://" + file.getAbsolutePath(); }
    }
    ReadCache(File parent) throws IOException {
        root = new File(parent.getCanonicalFile(), "cp-read");
        if ((!root.isDirectory() && !root.mkdirs()) || !root.getCanonicalFile().equals(root.getAbsoluteFile())) throw new IOException();
    }
    private File owned(File file) throws IOException {
        File canonical = file.getCanonicalFile();
        if (!canonical.getParentFile().equals(root) || !canonical.equals(file.getAbsoluteFile())) throw new IOException();
        return canonical;
    }
    synchronized void clear() throws IOException {
        File[] files = root.listFiles();
        if (files == null) throw new IOException();
        for (File file : files) if (file.getName().matches(ID + "\\.(?:" + EXT + ")")) {
            owned(file);
            if (!file.isFile() || !file.delete()) throw new IOException();
        }
        entries.clear();
    }
    synchronized String create(String version, String extension) throws IOException {
        if (!version.matches(ID) || !extension.matches(EXT) || entries.size() >= 4) throw new IOException();
        String handle = UUID.randomUUID().toString();
        File file = owned(new File(root, handle + "." + extension));
        if (!file.createNewFile()) throw new IOException();
        entries.put(handle, new Entry(file, extension));
        return handle;
    }
    private Entry entry(String handle) throws IOException {
        if (handle == null || !handle.matches(ID)) throw new IOException();
        Entry value = entries.get(handle);
        if (value == null || !owned(value.file).isFile()) throw new IOException();
        return value;
    }
    synchronized void append(String handle, String base64) throws IOException {
        Entry value = entry(handle);
        if (value.finished || base64 == null || base64.length() > ((MAX_CHUNK_BYTES + 2) / 3) * 4) throw new IOException();
        ByteString decoded = ByteString.decodeBase64(base64);
        if (decoded == null || !decoded.base64().equals(base64) || decoded.size() == 0 || decoded.size() > MAX_CHUNK_BYTES || value.bytes + decoded.size() > MAX_FILE_BYTES || value.file.length() != value.bytes) throw new IOException();
        byte[] bytes = decoded.toByteArray();
        try (FileOutputStream target = new FileOutputStream(value.file, true)) { target.write(bytes); value.bytes += bytes.length; }
        finally { Arrays.fill(bytes, (byte) 0); }
    }
    synchronized String finish(String handle, double bytes) throws IOException {
        Entry value = entry(handle);
        if (value.finished || Double.isNaN(bytes) || Double.isInfinite(bytes) || bytes != value.bytes || value.file.length() != value.bytes) throw new IOException();
        value.finished = true;
        return value.uri();
    }
    synchronized Entry byURI(String uri) throws IOException {
        for (Entry value : entries.values()) if (value.finished && value.uri().equals(uri)) { owned(value.file); if (value.file.length() != value.bytes) throw new IOException(); return value; }
        throw new IOException();
    }
    synchronized void remove(String handle) throws IOException {
        if (handle == null || !handle.matches(ID)) throw new IOException();
        Entry value = entries.get(handle);
        if (value == null) return;
        File file = owned(value.file);
        if (file.exists() && !file.delete()) throw new IOException();
        entries.remove(handle);
    }
    static String mime(String extension) throws IOException {
        switch (extension) {
            case "png": return "image/png";
            case "jpg": return "image/jpeg";
            case "gif": return "image/gif";
            case "webp": return "image/webp";
            case "csv": return "text/csv";
            case "pdf": return "application/pdf";
            case "doc": return "application/msword";
            case "docx": return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
            case "xls": return "application/vnd.ms-excel";
            case "xlsx": return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
            case "ppt": return "application/vnd.ms-powerpoint";
            case "pptx": return "application/vnd.openxmlformats-officedocument.presentationml.presentation";
            default: throw new IOException();
        }
    }
}
