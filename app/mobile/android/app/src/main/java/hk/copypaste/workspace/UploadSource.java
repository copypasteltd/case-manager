package hk.copypaste.workspace;

import java.io.DataOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.security.DigestOutputStream;
import java.security.MessageDigest;
import java.util.Arrays;

interface UploadSource {
    InputStream open(long offset, long length) throws IOException;
    void check() throws IOException;

    /** A bounded re-selection check; the server's full SHA-256 is authoritative. */
    static String fingerprint(UploadSource source, String name, long size, long modified) throws Exception {
        if (name == null || name.isEmpty() || name.length() > 255 || size < 1 || size > UploadLayout.MAX_BYTES || modified < 0) throw new IOException();
        source.check();
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (DataOutputStream metadata = new DataOutputStream(new DigestOutputStream(new java.io.OutputStream() {
            @Override public void write(int value) { }
            @Override public void write(byte[] value, int start, int length) { }
        }, digest))) {
            metadata.writeUTF(name); metadata.writeLong(size); metadata.writeLong(modified);
        }
        byte[] buffer = new byte[64 * 1024];
        try {
            long head = Math.min(buffer.length, size), tail = Math.min(buffer.length, size - head);
            for (long[] range : new long[][] {{0, head}, {size - tail, tail}}) {
                if (range[1] == 0) continue;
                try (InputStream input = source.open(range[0], range[1])) {
                    long remaining = range[1];
                    while (remaining > 0) {
                        int count = input.read(buffer, 0, (int) Math.min(buffer.length, remaining));
                        if (count <= 0) throw new IOException();
                        digest.update(buffer, 0, count); remaining -= count;
                    }
                }
            }
            source.check();
            StringBuilder result = new StringBuilder("android-saf-v1:");
            for (byte value : digest.digest()) result.append(String.format(java.util.Locale.ROOT, "%02x", value & 255));
            return result.toString();
        } finally { Arrays.fill(buffer, (byte) 0); }
    }
}
