package hk.copypaste.workspace;

import static org.junit.Assert.*;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.InetAddress;
import java.util.Arrays;
import java.util.Collections;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.RecordedRequest;
import okhttp3.mockwebserver.SocketPolicy;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;

public class UploadTransferTest {
    private MockWebServer server;
    private UploadTransfer transfer;
    @Before public void setup() throws Exception { server = new MockWebServer(); server.start(InetAddress.getByName("127.0.0.1"), 0); transfer = new UploadTransfer(); }
    @After public void teardown() throws Exception { transfer.close(); server.close(); }
    private UploadPermit permit() { return new UploadPermit(server.url("/draft?signature=transient-secret").toString(), Collections.singletonMap("Content-Type", "application/octet-stream")); }
    private UploadSource bytes(byte[] content) {
        return new UploadSource() {
            @Override public void check() { }
            @Override public InputStream open(long offset, long length) { return new ByteArrayInputStream(content, (int) offset, (int) length); }
        };
    }
    @Test public void actualPutUsesExactLongRangeWithoutBearerOrCookies() throws Exception {
        server.enqueue(new MockResponse().setHeader("ETag", "\"part-receipt\""));
        long offset = 3576L * 33_554_432; final long[] selected = {-1};
        UploadSource source = new UploadSource() { public void check() { } public InputStream open(long at, long length) { selected[0] = at; assertEquals(3, length); return new ByteArrayInputStream(new byte[] {7, 8, 9}); } };
        String etag = transfer.put(source, new UploadLayout(offset + 3, 33_554_432, 3577), 3577, permit(), value -> {});
        assertEquals("\"part-receipt\"", etag); assertEquals(offset, selected[0]);
        RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS); assertNotNull(request); assertEquals("PUT", request.getMethod());
        assertEquals("3", request.getHeader("Content-Length")); assertNull(request.getHeader("Authorization")); assertNull(request.getHeader("Cookie"));
        assertArrayEquals(new byte[] {7, 8, 9}, request.getBody().readByteArray());
    }
    @Test public void streamingBufferAndProgressRemainBounded() throws Exception {
        server.enqueue(new MockResponse().setHeader("ETag", "part")); final int[] max = {0}; final long[] total = {0};
        int size = 5 * 1024 * 1024 + 7;
        UploadSource source = new UploadSource() { public void check() { } public InputStream open(long offset, long length) { return new InputStream() {
            long remaining = length; public int read() { throw new AssertionError(); }
            public int read(byte[] target, int start, int count) { max[0] = Math.max(max[0], count); if (remaining == 0) return -1; int value = (int) Math.min(remaining, count); Arrays.fill(target, start, start + value, (byte) 21); remaining -= value; return value; }
        }; } };
        transfer.put(source, new UploadLayout(size, 32 * 1024 * 1024, 1), 1, permit(), value -> { assertTrue(value >= total[0]); total[0] = value; });
        assertEquals(65536, max[0]); assertEquals(size, total[0]); assertEquals(size, server.takeRequest().getBodySize());
    }
    @Test public void redirectIsNotFollowedOrAutomaticallyRetried() {
        server.enqueue(new MockResponse().setResponseCode(307).setHeader("Location", server.url("/other").toString()));
        assertThrows(IOException.class, () -> transfer.put(bytes(new byte[] {1}), new UploadLayout(1, 5_242_880, 1), 1, permit(), value -> {}));
        assertEquals(1, server.getRequestCount());
    }
    @Test public void providerErrorsAndSignedUrlsNeverEnterTheReportedError() {
        server.enqueue(new MockResponse().setResponseCode(503).setBody("cloud-private-error-and-key"));
        IOException error = assertThrows(IOException.class, () -> transfer.put(bytes(new byte[] {1}), new UploadLayout(1, 5_242_880, 1), 1, permit(), value -> {}));
        assertFalse(error.toString().contains("transient-secret")); assertFalse(error.toString().contains("cloud-private")); assertNull(error.getCause()); assertEquals(1, server.getRequestCount());
    }
    @Test public void missingReceiptCannotBeAcknowledged() {
        server.enqueue(new MockResponse());
        assertThrows(IOException.class, () -> transfer.put(bytes(new byte[] {1}), new UploadLayout(1, 5_242_880, 1), 1, permit(), value -> {}));
    }
    @Test public void anIncompleteSourceStopsBeforeAnySuccessReceipt() {
        server.enqueue(new MockResponse().setHeader("ETag", "part"));
        assertThrows(IOException.class, () -> transfer.put(bytes(new byte[] {1}), new UploadLayout(3, 5_242_880, 1), 1, permit(), value -> {}));
    }
    @Test public void changedSourceAfterCloudResponseCannotBeReportedAsConfirmed() {
        server.enqueue(new MockResponse().setHeader("ETag", "part"));
        UploadSource source = new UploadSource() { int checks; public void check() throws IOException { if (++checks > 1) throw new IOException(); } public InputStream open(long offset, long length) { return new ByteArrayInputStream(new byte[] {1}); } };
        assertThrows(IOException.class, () -> transfer.put(source, new UploadLayout(1, 5_242_880, 1), 1, permit(), value -> {}));
    }
    @Test public void cancellationStopsTheLiveRequestWithoutRetry() throws Exception {
        server.enqueue(new MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE)); ExecutorService executor = Executors.newSingleThreadExecutor();
        try {
            Future<?> result = executor.submit(() -> { assertThrows(IOException.class, () -> transfer.put(bytes(new byte[] {1}), new UploadLayout(1, 5_242_880, 1), 1, permit(), value -> {})); });
            assertNotNull(server.takeRequest(3, TimeUnit.SECONDS)); transfer.cancel(); result.get(5, TimeUnit.SECONDS); assertEquals(1, server.getRequestCount());
        } finally { executor.shutdownNow(); }
    }
}
