package hk.copypaste.workspace;

import com.facebook.react.ReactPackage;
import com.facebook.react.bridge.NativeModule;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.uimanager.ViewManager;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

public final class CPPlatformPackage implements ReactPackage {
    @Override public List<NativeModule> createNativeModules(ReactApplicationContext context) { return Arrays.asList(new CPHTTPModule(context), new CPReadCacheModule(context), new CPUploadModule(context), new CPClipboardModule(context)); }
    @Override public List<ViewManager> createViewManagers(ReactApplicationContext context) { return Collections.emptyList(); }
}
