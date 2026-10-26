package hk.copypaste.workspace;

import android.app.Application;
import android.content.res.Configuration;
import com.facebook.react.PackageList;
import com.facebook.react.ReactApplication;
import com.facebook.react.ReactHost;
import com.facebook.react.ReactNativeHost;
import com.facebook.react.ReactPackage;
import com.facebook.react.defaults.DefaultReactNativeHost;
import com.facebook.soloader.SoLoader;
import java.util.List;

public final class MainApplication extends Application implements ReactApplication {
    private final ReactNativeHost host = new DefaultReactNativeHost(this) {
        @Override public boolean getUseDeveloperSupport() { return false; }
        @Override protected List<ReactPackage> getPackages() {
            List<ReactPackage> packages = new PackageList(this).getPackages();
            packages.add(new CPPlatformPackage());
            return packages;
        }
        @Override protected String getJSMainModuleName() { return "index"; }
        @Override protected boolean isNewArchEnabled() { return false; }
        @Override protected Boolean isHermesEnabled() { return true; }
    };
    @Override public ReactNativeHost getReactNativeHost() { return host; }
    @Override public ReactHost getReactHost() { return null; }
    @Override public void onCreate() {
        super.onCreate();
        SoLoader.init(this, false);
    }
    @Override public void onConfigurationChanged(Configuration config) {
        super.onConfigurationChanged(config);
    }
}
