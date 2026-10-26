package hk.copypaste.workspace;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;
import androidx.core.app.NotificationCompat;
import androidx.core.app.ServiceCompat;

/** User-started dataSync service; never starts itself after boot or process death. */
public final class UploadServiceHost extends Service {
    private static final String CHANNEL = "cp-upload";
    private static final int NOTIFICATION = 507;
    private UploadManager manager;
    private String runID;
    private int lastStart;
    private long rendered;
    private final Runnable tick = new Runnable() {
        @Override public void run() {
            if (runID == null) return;
            manager.watchdog(runID); UploadManager.Run run = manager.active();
            if (run == null || !run.id.equals(runID)) { stopRun(runID); return; }
            if (android.os.SystemClock.elapsedRealtime() - rendered >= 1_000) {
                getSystemService(NotificationManager.class).notify(NOTIFICATION, notification(run)); rendered = android.os.SystemClock.elapsedRealtime();
            }
            manager.main.postDelayed(this, 1_000);
        }
    };
    @Override public void onCreate() {
        super.onCreate(); manager = UploadManager.get(this); manager.host = this;
        if (Build.VERSION.SDK_INT >= 26) getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel(CHANNEL, "文件上传", NotificationManager.IMPORTANCE_LOW));
    }
    @Override public int onStartCommand(Intent intent, int flags, int startId) {
        lastStart = startId; UploadManager.Run run = manager.active();
        if (intent == null || run == null || !run.id.equals(intent.getStringExtra("run"))) { stopSelfResult(startId); return START_NOT_STICKY; }
        if ("pause".equals(intent.getAction())) { manager.pause("paused"); stopRun(run.id); return START_NOT_STICKY; }
        runID = run.id;
        try {
            ServiceCompat.startForeground(this, NOTIFICATION, notification(run), Build.VERSION.SDK_INT >= 29 ? ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC : 0);
            manager.main.removeCallbacks(tick); manager.main.post(tick);
        } catch (Exception ignored) { manager.pause("paused"); stopSelfResult(startId); }
        return START_NOT_STICKY;
    }
    private Notification notification(UploadManager.Run run) {
        PendingIntent open = PendingIntent.getActivity(this, 0, new Intent(this, MainActivity.class), PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        PendingIntent pause = PendingIntent.getService(this, 1, new Intent(this, UploadServiceHost.class).setAction("pause").putExtra("run", run.id), PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
        int progress = (int) Math.min(10_000, run.confirmed * 10_000 / run.size);
        int sent = (int) Math.min(10_000, (run.transferred + run.sent) * 10_000 / run.size);
        return new NotificationCompat.Builder(this, CHANNEL).setSmallIcon(android.R.drawable.stat_sys_upload).setContentTitle("文件上传中")
            .setContentText("传输 " + sent / 100 + "% · 云端核对 " + progress / 100 + "%").setContentIntent(open).setOngoing(true).setOnlyAlertOnce(true)
            .setVisibility(NotificationCompat.VISIBILITY_SECRET).setCategory(NotificationCompat.CATEGORY_PROGRESS).setProgress(10_000, progress, false)
            .addAction(android.R.drawable.ic_media_pause, "暂停", pause).build();
    }
    void stopRun(String id) {
        UploadManager.Run current = manager.active();
        if (runID != null && runID.equals(id) && (current == null || current.id.equals(id))) {
            manager.main.removeCallbacks(tick); runID = null; ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE); stopSelfResult(lastStart);
        }
    }
    @Override public void onDestroy() {
        manager.main.removeCallbacks(tick);
        if (manager.host == this) manager.host = null;
        UploadManager.Run run = manager.active(); if (runID != null && run != null && runID.equals(run.id)) manager.pause("paused");
        super.onDestroy();
    }
    @Override public IBinder onBind(Intent intent) { return null; }
}
