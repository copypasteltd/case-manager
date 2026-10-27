package hk.copypaste.workspace;

import static org.junit.Assert.*;
import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;
import okio.ByteString;

public class ReadCacheTest {
    private static final String VERSION = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee";
    @Rule public TemporaryFolder directory = new TemporaryFolder();
    @Test public void completedContentUsesOnlyItsOwnedFileAndFixedFormat() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot()); String handle = cache.create(VERSION, "pdf");
        byte[] bytes = "owned fixed bytes".getBytes(StandardCharsets.UTF_8); cache.append(handle, ByteString.of(bytes).base64());
        assertThrows(IOException.class, () -> cache.byURI("file:///etc/passwd"));
        String uri = cache.finish(handle, bytes.length); ReadCache.Entry entry = cache.byURI(uri);
        assertEquals(new File(directory.getRoot(), "cp-read").getCanonicalFile(), entry.file.getCanonicalFile().getParentFile()); assertEquals("application/pdf", ReadCache.mime(entry.extension));
        assertThrows(IOException.class, () -> cache.append(handle, "YQ==")); assertThrows(IOException.class, () -> cache.finish(handle, bytes.length));
        cache.remove(handle); assertFalse(entry.file.exists()); cache.remove(handle);
    }
    @Test public void invalidPathsAndFormatsCannotCreateOrRemoveAnything() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot());
        assertThrows(IOException.class, () -> cache.create("../other", "pdf")); assertThrows(IOException.class, () -> cache.create(VERSION, "../../other")); assertThrows(IOException.class, () -> cache.create(VERSION, "html")); assertThrows(IOException.class, () -> cache.remove("../other"));
        assertEquals(0, new File(directory.getRoot(), "cp-read").listFiles().length);
    }
    @Test public void scratchCapacityAndCanonicalBase64AreEnforced() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot()); String first = cache.create(VERSION, "pdf");
        for (int index = 0; index < 3; index++) cache.create(VERSION, "pdf"); assertThrows(IOException.class, () -> cache.create(VERSION, "pdf"));
        for (String invalid : new String[]{"", "YQ", "YQ==\n", "Y?==", "YR=="}) assertThrows(IOException.class, () -> cache.append(first, invalid));
        cache.clear(); String handle = cache.create(VERSION, "pdf"); String chunk = ByteString.of(new byte[ReadCache.MAX_CHUNK_BYTES]).base64();
        for (int index = 0; index < ReadCache.MAX_FILE_BYTES / ReadCache.MAX_CHUNK_BYTES; index++) cache.append(handle, chunk);
        assertThrows(IOException.class, () -> cache.append(handle, "YQ==")); assertNotNull(cache.finish(handle, ReadCache.MAX_FILE_BYTES));
    }
    @Test public void corruptOrIncompleteScratchCannotBecomeAnOpenableArtifact() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot()); String handle = cache.create(VERSION, "pdf"); cache.append(handle, "YWJj");
        assertThrows(IOException.class, () -> cache.finish(handle, 2));
        File own = new File(directory.getRoot(), "cp-read/" + handle + ".pdf");
        try (FileOutputStream stream = new FileOutputStream(own, true)) { stream.write(1); }
        assertThrows(IOException.class, () -> cache.finish(handle, 3)); cache.remove(handle);
    }
    @Test public void verifiedCsvUsesPrivateScratchAndExactMime() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot()); String handle = cache.create(VERSION, "csv");
        cache.append(handle, "77u/YWJjDQo="); String uri = cache.finish(handle, 8);
        assertEquals("text/csv", ReadCache.mime(cache.byURI(uri).extension));
        assertEquals(8, cache.byURI(uri).file.length()); cache.remove(handle);
        assertThrows(IOException.class, () -> cache.byURI(uri));
    }
    @Test public void processRestartCannotAdoptAnOldHandleAndCleanupPreservesUnownedFiles() throws Exception {
        ReadCache cache = new ReadCache(directory.getRoot()); String handle = cache.create(VERSION, "pdf"); String uri = cache.finish(handle, 0);
        File unrelated = new File(directory.getRoot(), "cp-read/unrelated.keep"); assertTrue(unrelated.createNewFile());
        ReadCache restarted = new ReadCache(directory.getRoot()); assertThrows(IOException.class, () -> restarted.byURI(uri)); restarted.clear();
        assertTrue(unrelated.isFile()); assertFalse(new File(directory.getRoot(), "cp-read/" + handle + ".pdf").exists());
    }
}
