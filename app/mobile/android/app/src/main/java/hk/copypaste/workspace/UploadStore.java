package hk.copypaste.workspace;

import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.sqlite.SQLiteDatabase;
import android.database.sqlite.SQLiteOpenHelper;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;
import org.json.JSONObject;

/** Private metadata only. No bearer, signed address, headers, ETag or file bytes. */
final class UploadStore extends SQLiteOpenHelper {
    static final int MAX_JOBS = 16;
    static final class Record {
        String id, owner, uri, name, media, fingerprint, upload, createKey, createBody, state;
        long size, modified, partBytes, storageEpoch, preparedAt;
        int partCount;
        JSONObject publicValue() throws Exception {
            return new JSONObject().put("handle", id).put("name", name).put("media_type", media)
                .put("size_bytes", Long.toString(size)).put("fingerprint", fingerprint).put("upload_id", upload)
                .put("create_key", createKey).put("create_body", createBody).put("prepared_at_ms", preparedAt).put("state", state).put("has_source", !uri.isEmpty());
        }
    }
    UploadStore(Context context) { super(context, "upload-sources.db", null, 1); }
    static String id(String value) { if (value == null || !value.matches("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")) throw new IllegalArgumentException(); return value; }
    @Override public void onCreate(SQLiteDatabase db) {
        db.execSQL("CREATE TABLE sources(id TEXT PRIMARY KEY,owner TEXT NOT NULL,uri TEXT NOT NULL,name TEXT NOT NULL,media TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL,fingerprint TEXT NOT NULL,upload TEXT NOT NULL DEFAULT '',create_key TEXT NOT NULL,create_body TEXT NOT NULL DEFAULT '',state TEXT NOT NULL,part_bytes INTEGER NOT NULL DEFAULT 0,part_count INTEGER NOT NULL DEFAULT 0,storage_epoch INTEGER NOT NULL DEFAULT 0,prepared_at INTEGER NOT NULL DEFAULT 0)");
    }
    @Override public void onUpgrade(SQLiteDatabase db, int oldVersion, int newVersion) { throw new IllegalStateException("Explicit upload metadata migration required."); }
    private static Record record(Cursor cursor) {
        Record record = new Record(); int index = 0;
        record.id = cursor.getString(index++); record.owner = cursor.getString(index++); record.uri = cursor.getString(index++);
        record.name = cursor.getString(index++); record.media = cursor.getString(index++); record.size = cursor.getLong(index++);
        record.modified = cursor.getLong(index++); record.fingerprint = cursor.getString(index++); record.upload = cursor.getString(index++);
        record.createKey = cursor.getString(index++); record.createBody = cursor.getString(index++); record.state = cursor.getString(index++);
        record.partBytes = cursor.getLong(index++); record.partCount = cursor.getInt(index++); record.storageEpoch = cursor.getLong(index++); record.preparedAt = cursor.getLong(index);
        return record;
    }
    synchronized List<Record> list(String owner) {
        List<Record> result = new ArrayList<>();
        try (Cursor cursor = getReadableDatabase().query("sources", null, "owner=?", new String[] {id(owner)}, null, null, "rowid DESC", Integer.toString(MAX_JOBS))) {
            while (cursor.moveToNext()) result.add(record(cursor));
        }
        return result;
    }
    synchronized Record get(String owner, String handle) {
        try (Cursor cursor = getReadableDatabase().query("sources", null, "owner=? AND id=?", new String[] {id(owner), id(handle)}, null, null, null)) {
            if (!cursor.moveToFirst()) throw new IllegalArgumentException(); return record(cursor);
        }
    }
    synchronized Record add(String owner, String uri, SAFUploadSource source, String fingerprint) {
        try (Cursor count = getReadableDatabase().rawQuery("SELECT count(*),sum(CASE WHEN owner=? THEN 1 ELSE 0 END) FROM sources", new String[] {id(owner)})) {
            if (!count.moveToFirst() || count.getInt(0) >= 64 || count.getInt(1) >= MAX_JOBS) throw new IllegalStateException();
        }
        String handle = UUID.randomUUID().toString(); ContentValues values = new ContentValues();
        values.put("id", handle); values.put("owner", id(owner)); values.put("uri", uri); values.put("name", source.name);
        values.put("media", source.media); values.put("size", source.size); values.put("modified", source.modified);
        values.put("fingerprint", fingerprint); values.put("create_key", "android-upload:" + UUID.randomUUID()); values.put("state", "selected");
        getWritableDatabase().insertOrThrow("sources", null, values); return get(owner, handle);
    }
    synchronized void update(String owner, String handle, ContentValues values) {
        if (getWritableDatabase().update("sources", values, "owner=? AND id=?", new String[] {id(owner), id(handle)}) != 1) throw new IllegalArgumentException();
    }
    synchronized void state(String owner, String handle, String state) { ContentValues values = new ContentValues(); values.put("state", state); update(owner, handle, values); }
    synchronized void delete(String owner, String handle) { getWritableDatabase().delete("sources", "owner=? AND id=?", new String[] {id(owner), id(handle)}); }
    synchronized boolean hasURI(String uri) {
        try (Cursor cursor = getReadableDatabase().rawQuery("SELECT 1 FROM sources WHERE uri=? LIMIT 1", new String[] {uri})) { return cursor.moveToFirst(); }
    }
}
