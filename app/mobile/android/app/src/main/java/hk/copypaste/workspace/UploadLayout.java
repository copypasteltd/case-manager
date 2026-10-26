package hk.copypaste.workspace;

import java.util.HashSet;
import java.util.Set;

/** All offsets remain long; no file or part is materialized in memory. */
final class UploadLayout {
    static final long MAX_BYTES = 120_000_000_000L;
    final long size, partBytes;
    final int count;
    UploadLayout(long size, long partBytes, int count) {
        if (size < 1 || size > MAX_BYTES || partBytes < 5L * 1024 * 1024 || partBytes > 512L * 1024 * 1024 ||
            count < 1 || count > 10_000 || count != (size + partBytes - 1) / partBytes) throw new IllegalArgumentException();
        this.size = size; this.partBytes = partBytes; this.count = count;
    }
    long start(int number) { check(number); return (number - 1L) * partBytes; }
    long length(int number) { return Math.min(partBytes, size - start(number)); }
    private void check(int number) { if (number < 1 || number > count) throw new IllegalArgumentException(); }
    long confirmed(int[] parts) {
        Set<Integer> seen = new HashSet<>(); long total = 0;
        for (int number : parts) { if (!seen.add(number)) throw new IllegalArgumentException(); total += length(number); }
        return total;
    }
}
