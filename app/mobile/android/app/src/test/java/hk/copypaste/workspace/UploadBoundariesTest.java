package hk.copypaste.workspace;

import static org.junit.Assert.*;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.junit.Test;

public class UploadBoundariesTest {
    @Test public void fullCapacityUsesLongOffsetsAndExactLastPart() {
        UploadLayout layout = new UploadLayout(120_000_000_000L, 32L * 1024 * 1024, 3577);
        assertEquals(119_990_648_832L, layout.start(3577)); assertEquals(9_351_168L, layout.length(3577));
        assertEquals(42_905_600L, layout.confirmed(new int[] {1, 3577}));
    }
    @Test public void malformedLayoutsAndDuplicatedProgressAreRejected() {
        for (long[] values : new long[][] {{0, 5_242_880, 1}, {120_000_000_001L, 33_554_432, 3577}, {10, 2, 5}, {10, 536_870_913, 1}, {20_000_000, 5_242_880, 3}})
            assertThrows(IllegalArgumentException.class, () -> new UploadLayout(values[0], values[1], (int) values[2]));
        UploadLayout layout = new UploadLayout(1, 5_242_880, 1);
        assertThrows(IllegalArgumentException.class, () -> layout.confirmed(new int[] {1, 1}));
        assertThrows(IllegalArgumentException.class, () -> layout.start(0)); assertThrows(IllegalArgumentException.class, () -> layout.length(2));
    }
    @Test public void fingerprintSamplesOnlyHeadAndTailAtFullCapacity() throws Exception {
        List<long[]> reads = new ArrayList<>(); final long[] total = {0};
        UploadSource source = new UploadSource() {
            @Override public void check() { }
            @Override public InputStream open(long offset, long length) {
                reads.add(new long[] {offset, length});
                return new InputStream() {
                    long remaining = length;
                    @Override public int read() { throw new AssertionError(); }
                    @Override public int read(byte[] buffer, int start, int count) { assertTrue(count <= 65536); int value = (int) Math.min(count, remaining); if (value == 0) return -1; Arrays.fill(buffer, start, start + value, (byte) 42); remaining -= value; total[0] += value; return value; }
                };
            }
        };
        String fingerprint = UploadSource.fingerprint(source, "archive.bin", 120_000_000_000L, 123);
        assertTrue(fingerprint.matches("android-saf-v1:[a-f0-9]{64}")); assertEquals(131_072, total[0]);
        assertArrayEquals(new long[] {0, 65536}, reads.get(0)); assertArrayEquals(new long[] {119_999_934_464L, 65536}, reads.get(1));
    }
    @Test public void officeFingerprintChecksAllBytesAndRejectsShortOrOversizedSources() throws Exception {
        byte[] bytes=new byte[196608]; Arrays.fill(bytes,(byte)42);
        UploadSource source=new UploadSource(){public void check(){} public InputStream open(long offset,long length){assertEquals(0,offset);assertEquals(bytes.length,length);return new ByteArrayInputStream(bytes);}};
        String first=UploadSource.officeFingerprint(source,bytes.length);
        assertTrue(first.matches("office-artifact-v1:[a-f0-9]{64}"));
        bytes[100000]^=1; assertNotEquals(first,UploadSource.officeFingerprint(source,bytes.length));
        UploadSource shortSource=new UploadSource(){public void check(){} public InputStream open(long o,long l){return new ByteArrayInputStream(new byte[1]);}};
        assertThrows(IOException.class,()->UploadSource.officeFingerprint(shortSource,10));
        assertThrows(IOException.class,()->UploadSource.officeFingerprint(source,12L*1024*1024+1));
    }
    @Test public void shortOrChangedSourcesCannotProduceAValidFingerprint() {
        UploadSource shortSource = new UploadSource() { public void check() { } public InputStream open(long offset, long length) { return new ByteArrayInputStream(new byte[0]); } };
        assertThrows(IOException.class, () -> UploadSource.fingerprint(shortSource, "a.bin", 10, 0));
        UploadSource changed = new UploadSource() { int checks; public void check() throws IOException { if (++checks > 1) throw new IOException(); } public InputStream open(long offset, long length) { return new ByteArrayInputStream(new byte[(int) length]); } };
        assertThrows(IOException.class, () -> UploadSource.fingerprint(changed, "a.bin", 10, 0));
    }
    @Test public void onlyHttpsProviderCapabilitiesAndSafeHeadersAreAccepted() {
        long now = 1_000_000;
        for (String url : new String[] {"https://bucket.oss-cn-hongkong.aliyuncs.com/draft?signature=private", "https://bucket.account.r2.cloudflarestorage.com/draft?signature=private"}) {
            UploadPermit permit = UploadPermit.validate(url, "PUT", 2, 2, now + 300_000, now, Collections.singletonMap("x-oss-meta-check", "okay")); assertEquals(url, permit.url);
        }
        for (String url : new String[] {"http://bucket.aliyuncs.com/x", "https://aliyuncs.com.evil.example/x", "https://user@bucket.aliyuncs.com/x", "https://bucket.aliyuncs.com:8443/x", "https://bucket.aliyuncs.com/x#secret", "https://127.0.0.1/x"})
            assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "PUT", 1, 1, now + 60_000, now, Collections.emptyMap()));
        for (String header : new String[] {"Authorization", "Cookie", "Host", "Content-Length", "Referer"})
            assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate("https://b.aliyuncs.com/x", "PUT", 1, 1, now + 60_000, now, Collections.singletonMap(header, "private")));
    }
    @Test public void expiredWrongPartAndConflictingHeadersAreRejected() {
        String url = "https://bucket.aliyuncs.com/draft"; long now = 1_000_000;
        for (long expires : new long[] {now, now + 9999, now + 330_001}) assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "PUT", 1, 1, expires, now, Collections.emptyMap()));
        assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "POST", 1, 1, now + 60_000, now, Collections.emptyMap()));
        assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "PUT", 2, 1, now + 60_000, now, Collections.emptyMap()));
        Map<String, String> headers = new HashMap<>(); headers.put("X-Oss-A", "one"); headers.put("x-oss-a", "two");
        assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "PUT", 1, 1, now + 60_000, now, headers));
        assertThrows(IllegalArgumentException.class, () -> UploadPermit.validate(url, "PUT", 1, 1, now + 60_000, now, Collections.singletonMap("content-type", "x\r\nsecret")));
    }
}
