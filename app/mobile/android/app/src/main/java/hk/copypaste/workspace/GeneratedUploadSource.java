package hk.copypaste.workspace;

import android.content.Context;
import android.net.Uri;
import android.os.CancellationSignal;
import android.util.Base64;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Arrays;
import java.util.UUID;

/** Bounded Office artifacts only. Ordinary large files stay on the SAF path. */
final class GeneratedUploadSource extends SAFUploadSource {
    static final long MAX_BYTES=12L*1024*1024;
    private final File file;
    private GeneratedUploadSource(File file, Uri uri, String name, String media, long size, long modified, CancellationSignal signal) throws IOException {
        super(uri,name,media,size,modified,signal);this.file=file;check();
    }
    static File resolve(Context context,String uri) throws IOException {
        if (uri==null||!uri.matches("cp-generated:[a-f0-9-]{36}:[a-f0-9-]{36}")) throw new IOException();
        String[] ids=uri.split(":");UploadStore.id(ids[1]);UploadStore.id(ids[2]);
        File root=new File(context.getFilesDir(),"office-upload-sources").getCanonicalFile();
        File dest=new File(new File(root,ids[1]),ids[2]+".bin").getCanonicalFile();
        if (!dest.getPath().startsWith(root.getPath()+File.separator)) throw new IOException();return dest;
    }
    static GeneratedUploadSource open(Context context, UploadStore.Record record, CancellationSignal signal) throws Exception {
        String[] ids=record.uri.split(":");if(ids.length!=3||!ids[1].equals(record.owner)) throw new IOException();
        return new GeneratedUploadSource(resolve(context,record.uri),Uri.parse(record.uri),record.name,record.media,record.size,record.modified,signal);
    }
    static GeneratedUploadSource create(Context context,String owner,String name,String media,String encoded,String sha) throws Exception {
        UploadStore.id(owner);
        if (name==null||name.isEmpty()||name.length()>200||name.contains("/")||name.contains("\\")||!sha.matches("[a-f0-9]{64}")||encoded.length()>16*1024*1024||encoded.length()%4!=0||!encoded.matches("[A-Za-z0-9+/]*={0,2}")) throw new IOException();
        if (!(media.equals("application/vnd.openxmlformats-officedocument.wordprocessingml.document")||media.equals("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")||media.equals("application/vnd.openxmlformats-officedocument.presentationml.presentation"))) throw new IOException();
        byte[] bytes=Base64.decode(encoded,Base64.NO_WRAP);String uri="cp-generated:"+owner+":"+UUID.randomUUID();File file=resolve(context,uri);
        try {
            if(bytes.length<1||bytes.length>MAX_BYTES||!sha.equals(hex(MessageDigest.getInstance("SHA-256").digest(bytes)))||!Base64.encodeToString(bytes,Base64.NO_WRAP).equals(encoded)) throw new IOException();
            if(!file.getParentFile().isDirectory()&&!file.getParentFile().mkdirs()||!file.createNewFile()) throw new IOException();
            try(FileOutputStream out=new FileOutputStream(file)){out.write(bytes);out.getFD().sync();}
            return new GeneratedUploadSource(file,Uri.parse(uri),name,media,bytes.length,file.lastModified(),new CancellationSignal());
        } catch(Exception error){file.delete();throw error;} finally {Arrays.fill(bytes,(byte)0);}
    }
    static String hex(byte[] bytes){StringBuilder out=new StringBuilder();for(byte value:bytes)out.append(String.format(java.util.Locale.ROOT,"%02x",value&255));return out.toString();}
    String fingerprint() throws Exception {
        MessageDigest hash=MessageDigest.getInstance("SHA-256");byte[] buffer=new byte[65536];
        try(InputStream in=open(0,size)){long count=0;int read;while((read=in.read(buffer))!=-1){count+=read;if(count>size)throw new IOException();hash.update(buffer,0,read);}if(count!=size)throw new IOException();}
        finally{Arrays.fill(buffer,(byte)0);}check();return "office-artifact-v1:"+hex(hash.digest());
    }
    @Override public void check() throws IOException {if(cancellation.isCanceled()||!file.isFile()||size<1||size>MAX_BYTES||file.length()!=size||file.lastModified()!=modified)throw new IOException();}
    @Override public InputStream open(long offset,long length) throws IOException {
        check();if(offset<0||length<0||offset>size||length>size-offset)throw new IOException();
        FileInputStream in=new FileInputStream(file);try{in.getChannel().position(offset);return new java.io.FilterInputStream(in){long left=length;
            @Override public int read(byte[] bytes,int start,int count)throws IOException{check();if(left==0)return -1;int n=super.read(bytes,start,(int)Math.min(count,left));if(n>0)left-=n;return n;}
            @Override public int read()throws IOException{byte[] one=new byte[1];return read(one,0,1)<0?-1:one[0]&255;}
        };}catch(Exception e){in.close();throw e;}
    }
}
