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
    @Test public void fingerprintReadsEveryByteWithBoundedBuffersAndDetectsMiddleChanges() throws Exception {
        byte[] bytes=new byte[3*1024*1024]; final long[] total={0}; final int[] max={0};
        UploadSource source=new UploadSource(){ public void check(){} public InputStream open(long offset,long length){
            assertEquals(0,offset);assertEquals(bytes.length,length);
            return new ByteArrayInputStream(bytes){ @Override public synchronized int read(byte[] b,int start,int count){
                max[0]=Math.max(max[0],count);int n=super.read(b,start,count);if(n>0)total[0]+=n;return n;
            }};
        }};
        String first=UploadSource.fingerprint(source,"same.bin",bytes.length,123);
        assertTrue(first.matches("source-sha256-v2:[a-f0-9]{64}"));assertEquals(bytes.length,total[0]);assertTrue(max[0]<=65536);
        bytes[1024*1024+17]=1;
        assertNotEquals(first,UploadSource.fingerprint(source,"same.bin",bytes.length,123));
        UploadSource abc=new UploadSource(){public void check(){} public InputStream open(long o,long n){return new ByteArrayInputStream(new byte[]{97,98,99});}};
        assertEquals("source-sha256-v2:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",UploadSource.fingerprint(abc,"abc",3,0));
    }
    @Test public void fingerprintCancelsWithoutReadingFullCapacity() throws Exception {
        UploadSource source=new UploadSource(){public void check(){} public InputStream open(long o,long n){assertEquals(120_000_000_000L,n);return new ByteArrayInputStream(new byte[1]);}};
        Thread.currentThread().interrupt();
        try {assertThrows(java.io.InterruptedIOException.class,()->UploadSource.fingerprint(source,"large.bin",120_000_000_000L,0));}
        finally {Thread.interrupted();}
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
