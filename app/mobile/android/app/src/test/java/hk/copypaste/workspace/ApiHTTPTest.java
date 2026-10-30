package hk.copypaste.workspace;

import static org.junit.Assert.*;
import java.net.URI;
import java.net.InetAddress;
import java.util.Collections;
import java.util.HashMap;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import okhttp3.mockwebserver.MockResponse;
import okhttp3.mockwebserver.MockWebServer;
import okhttp3.mockwebserver.SocketPolicy;
import okio.Buffer;
import org.junit.Test;

public class ApiHTTPTest {
    @Test public void officeArtifactsHaveAnExplicitBoundedExceptionOnlyAtTheExactRenderRoute() throws Exception {
        String route="/v1/files/11111111-1111-1111-1111-111111111111/office-render";
        assertEquals(18*1024*1024,ApiHTTP.responseLimit("POST",route));
        for(String path:new String[]{route+"?x=1",route+"/other","/v1/me","/v1/files/not-uuid/office-render"})assertEquals(ApiHTTP.MAX_RESPONSE_BYTES,ApiHTTP.responseLimit("POST",path));
        assertEquals(ApiHTTP.MAX_RESPONSE_BYTES,ApiHTTP.responseLimit("GET",route));
        try(MockWebServer server=new MockWebServer()) {
            start(server);server.enqueue(json("{\"data\":\""+String.join("",Collections.nCopies(3*1024*1024,"x"))+"\"}"));
            ApiHTTP api=new ApiHTTP(origin(server),true);try{Result result=new Result();api.request("1","POST",route,Collections.emptyMap(),"{}",result);result.await();assertEquals(200,result.status);assertEquals(0,result.failures);}finally{api.close();}
        }
    }
    static class Result implements ApiHTTP.Result {
        final CountDownLatch done = new CountDownLatch(1); int status; String body; int failures;
        public void complete(int value, String data) { status = value; body = data; done.countDown(); }
        public void failed() { failures++; done.countDown(); }
        void await() throws Exception { assertTrue("Native call must finish", done.await(5, TimeUnit.SECONDS)); }
    }
    private static void start(MockWebServer server) throws Exception { server.start(InetAddress.getByName("127.0.0.1"), 0); }
    private static String origin(MockWebServer server) { return "http://127.0.0.1:" + server.getPort(); }
    private static MockResponse json(String body) { return new MockResponse().setHeader("Content-Type", "application/json").setBody(body); }

    @Test public void originAndRelativePathNeverEscapeTheConfiguredAPI() {
        for (String bad : new String[]{"http://api.example", "https://u:p@api.example", "https://api.example/path", "https://api.example?x", "https://api.example#x", "https://api.example:0", "https://api.example:99999"}) assertThrows(IllegalArgumentException.class, () -> ApiHTTP.validateOrigin(bad, false));
        assertThrows(IllegalArgumentException.class, () -> ApiHTTP.validateOrigin("http://192.0.2.1", true));
        ApiHTTP api = new ApiHTTP("https://api.example", false);
        try {
            assertEquals(URI.create("https://api.example/v1/me?x=1"), api.target("/v1/me?x=1"));
            for (String bad : new String[]{"https://other.example/v1/me", "//other.example/v1/me", "/v1/../../private", "/v1/%2e%2e/private", "/v1/a%2f..%2f..%2fprivate", "/v1/me#token", "/v1/\\other", "/v1/%5cother", "/v1/a%00b"}) assertThrows(IllegalArgumentException.class, () -> api.target(bad));
        } finally { api.close(); }
    }
    @Test public void actualJSONAndFixedVersionHeadersReachOnlyTheOriginalPath() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            start(server); server.enqueue(json("{\"data\":{\"ok\":true}}"));
            ApiHTTP api = new ApiHTTP(origin(server), true);
            try {
                Map<String, String> headers = new HashMap<>(); headers.put("Authorization", "Bearer owned-test-token"); headers.put("If-Range", "\"" + String.join("", Collections.nCopies(64, "a")) + "\"");
                Result result = new Result(); api.request("1", "GET", "/v1/downloads/test/chunks?offset=0&length=12", headers, null, result); result.await();
                assertEquals(200, result.status); assertEquals(0, result.failures);
                okhttp3.mockwebserver.RecordedRequest request = server.takeRequest(2, TimeUnit.SECONDS);
                assertEquals(headers.get("If-Range"), request.getHeader("If-Range")); assertEquals(headers.get("Authorization"), request.getHeader("Authorization")); assertEquals("/v1/downloads/test/chunks?offset=0&length=12", request.getPath());
            } finally { api.close(); }
        }
    }
    @Test public void redirectCannotForwardCredentialsOrBody() throws Exception {
        try (MockWebServer server = new MockWebServer(); MockWebServer other = new MockWebServer()) {
            start(server); start(other); server.enqueue(new MockResponse().setResponseCode(307).setHeader("Location", other.url("/received")));
            ApiHTTP api = new ApiHTTP(origin(server), true);
            try { Result result = new Result(); api.request("1", "POST", "/v1/auth/login", Collections.emptyMap(), "{\"password\":\"owned-test\"}", result); result.await(); assertEquals(1, result.failures); assertEquals(0, other.getRequestCount()); }
            finally { api.close(); }
        }
    }
    @Test public void oversizedAndInvalidUTF8BodiesNeverReachJSAsSuccess() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            start(server);
            server.enqueue(new MockResponse().setHeader("Content-Type", "application/json").setChunkedBody(new Buffer().write(new byte[ApiHTTP.MAX_RESPONSE_BYTES + 1]), 8192));
            server.enqueue(new MockResponse().setHeader("Content-Type", "application/json").setBody(new Buffer().writeByte(0xc3)));
            ApiHTTP api = new ApiHTTP(origin(server), true);
            try { for (int index = 1; index <= 2; index++) { Result result = new Result(); api.request(String.valueOf(index), "GET", "/v1/me", Collections.emptyMap(), null, result); result.await(); assertEquals(1, result.failures); assertNull(result.body); } }
            finally { api.close(); }
        }
    }
    @Test public void cancellationStopsItsCallAndDisconnectedWritesAreNotRetried() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            start(server); server.enqueue(new MockResponse().setSocketPolicy(SocketPolicy.NO_RESPONSE));
            ApiHTTP api = new ApiHTTP(origin(server), true);
            try {
                Result first = new Result(); api.request("1", "GET", "/v1/me", Collections.emptyMap(), null, first); assertNotNull(server.takeRequest(2, TimeUnit.SECONDS)); api.cancel("1"); first.await(); assertEquals(1, first.failures);
                server.enqueue(new MockResponse().setSocketPolicy(SocketPolicy.DISCONNECT_AT_START));
                Result second = new Result(); api.request("2", "POST", "/v1/requests", Collections.singletonMap("Idempotency-Key", "owned-test-intent"), "{}", second); second.await(); assertEquals(1, second.failures); assertEquals(2, server.getRequestCount());
            } finally { api.close(); }
        }
    }
    @Test public void invalidHeadersAndOversizedCommandsFailBeforeNetwork() throws Exception {
        try (MockWebServer server = new MockWebServer()) {
            start(server); ApiHTTP api = new ApiHTTP(origin(server), true);
            try {
                Result header = new Result(); api.request("1", "GET", "/v1/me", Collections.singletonMap("Cookie", "forbidden"), null, header); header.await(); assertEquals(1, header.failures);
                Result range = new Result(); api.request("2", "GET", "/v1/me", Collections.singletonMap("If-Range", "not-an-exact-version"), null, range); range.await(); assertEquals(1, range.failures);
                Result large = new Result(); api.request("3", "POST", "/v1/requests", Collections.emptyMap(), new String(new char[ApiHTTP.MAX_REQUEST_BYTES + 1]), large); large.await(); assertEquals(1, large.failures); assertEquals(0, server.getRequestCount());
            } finally { api.close(); }
        }
    }
}
