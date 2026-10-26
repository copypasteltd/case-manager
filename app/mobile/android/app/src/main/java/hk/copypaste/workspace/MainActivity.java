package hk.copypaste.workspace;

import android.os.Bundle;
import android.view.WindowManager;
import com.facebook.react.ReactActivity;
import com.facebook.react.ReactActivityDelegate;
import com.facebook.react.defaults.DefaultReactActivityDelegate;

public final class MainActivity extends ReactActivity {
    @Override protected String getMainComponentName() { return "CopyPasteWorkspace"; }
    @Override protected void onCreate(Bundle state) {
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
        // Activity restoration must not recreate an old authenticated route.
        super.onCreate(null);
    }
    @Override protected ReactActivityDelegate createReactActivityDelegate() {
        return new DefaultReactActivityDelegate(this, getMainComponentName(), false);
    }
}
