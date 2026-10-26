package hk.copypaste.workspace;

import android.content.ContentResolver;
import android.database.Cursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.os.CancellationSignal;
import android.provider.DocumentsContract;
import java.io.FileInputStream;
import java.io.FilterInputStream;
import java.io.IOException;
import java.io.InputStream;

/** Only system-picked, persistably granted, seekable documents are accepted. */
final class SAFUploadSource implements UploadSource {
    final ContentResolver resolver;
    final Uri uri;
    final String name, media;
    final long size, modified;
    final CancellationSignal cancellation;
    SAFUploadSource(ContentResolver resolver, Uri uri) throws IOException {
        this(resolver, uri, new CancellationSignal());
    }
    SAFUploadSource(ContentResolver resolver, Uri uri, CancellationSignal cancellation) throws IOException {
        this.resolver = resolver; this.uri = uri; this.cancellation = cancellation;
        String[] fields = {DocumentsContract.Document.COLUMN_DISPLAY_NAME, DocumentsContract.Document.COLUMN_SIZE,
            DocumentsContract.Document.COLUMN_LAST_MODIFIED, DocumentsContract.Document.COLUMN_FLAGS};
        try (Cursor cursor = resolver.query(uri, fields, null, null, null, cancellation)) {
            if (cursor == null || !cursor.moveToFirst() || cursor.isNull(0) || cursor.isNull(1) ||
                (cursor.getLong(3) & DocumentsContract.Document.FLAG_VIRTUAL_DOCUMENT) != 0) throw new IOException();
            name = cursor.getString(0); size = cursor.getLong(1); modified = cursor.isNull(2) ? 0 : cursor.getLong(2);
        } catch (Exception ignored) { throw new IOException("Select a seekable document with a known size."); }
        if (name == null || name.isEmpty() || name.length() > 255 || size < 1 || size > UploadLayout.MAX_BYTES || modified < 0) throw new IOException();
        String type = resolver.getType(uri); media = type != null && type.matches("[a-zA-Z0-9!#$&^_.+-]+/[a-zA-Z0-9!#$&^_.+-]+") ? type : "application/octet-stream";
        check();
    }
    @Override public void check() throws IOException {
        try (InputStream ignored = open(0, size)) { }
    }
    private void grant() throws IOException {
        for (android.content.UriPermission permission : resolver.getPersistedUriPermissions()) if (permission.isReadPermission() && uri.equals(permission.getUri())) return;
        throw new IOException("The document permission is no longer available.");
    }
    @Override public InputStream open(long offset, long length) throws IOException {
        if (offset < 0 || length < 0 || offset > size || length > size - offset) throw new IOException();
        ParcelFileDescriptor descriptor = null;
        try {
            grant();
            cancellation.throwIfCanceled(); descriptor = resolver.openFileDescriptor(uri, "r", cancellation);
            if (descriptor == null || descriptor.getStatSize() != size) throw new IOException();
            // AutoCloseInputStream owns the descriptor; no full-file copy or Blob.
            FileInputStream input = new ParcelFileDescriptor.AutoCloseInputStream(descriptor);
            try {
                if (input.getChannel().size() != size) throw new IOException();
                input.getChannel().position(offset);
                return new FilterInputStream(input) {
                    long checked = android.os.SystemClock.elapsedRealtime();
                    @Override public int read(byte[] buffer, int offset, int count) throws IOException {
                        if (cancellation.isCanceled()) throw new IOException();
                        long now = android.os.SystemClock.elapsedRealtime();
                        if (now - checked >= 1_000) { grant(); checked = now; }
                        return in.read(buffer, offset, count);
                    }
                };
            } catch (Exception failure) { input.close(); throw failure; }
        } catch (Exception ignored) {
            if (descriptor != null) try { descriptor.close(); } catch (IOException failure) { }
            throw new IOException("File access changed or random reads are unavailable.");
        }
    }
}
