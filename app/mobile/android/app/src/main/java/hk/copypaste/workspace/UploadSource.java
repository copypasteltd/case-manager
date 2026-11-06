package hk.copypaste.workspace;

import java.io.IOException;
import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Arrays;

interface UploadSource {
    InputStream open(long offset, long length) throws IOException;
    void check() throws IOException;
    static String officeFingerprint(UploadSource source,long size) throws Exception {
        if(size<1||size>12L*1024*1024)throw new IOException();source.check();
        MessageDigest hash=MessageDigest.getInstance("SHA-256");byte[] buffer=new byte[65536];
        try(InputStream in=source.open(0,size)){long count=0;int read;while((read=in.read(buffer))!=-1){count+=read;if(count>size)throw new IOException();hash.update(buffer,0,read);}if(count!=size)throw new IOException();}
        finally{Arrays.fill(buffer,(byte)0);}source.check();StringBuilder out=new StringBuilder("office-artifact-v1:");for(byte value:hash.digest())out.append(String.format(java.util.Locale.ROOT,"%02x",value&255));return out.toString();
    }

    /** Complete content identity, bounded memory and interruptible native IO. */
    static String fingerprint(UploadSource source, String name, long size, long modified) throws Exception {
        if (name == null || name.isEmpty() || name.length() > 255 || size < 1 || size > UploadLayout.MAX_BYTES || modified < 0) throw new IOException();
        source.check();
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] buffer = new byte[64 * 1024];
        try (InputStream input = source.open(0, size)) {
            long remaining = size;
            while (remaining > 0) {
                if (Thread.currentThread().isInterrupted()) throw new java.io.InterruptedIOException();
                int count = input.read(buffer, 0, (int)Math.min(buffer.length, remaining));
                if (count <= 0) throw new IOException();
                digest.update(buffer, 0, count); remaining -= count;
            }
            if (input.read() != -1) throw new IOException();
            source.check();
            StringBuilder result = new StringBuilder("source-sha256-v2:");
            for (byte value : digest.digest()) result.append(String.format(java.util.Locale.ROOT, "%02x", value & 255));
            return result.toString();
        } finally { Arrays.fill(buffer, (byte)0); }
    }
}
