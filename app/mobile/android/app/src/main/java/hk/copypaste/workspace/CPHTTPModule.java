package hk.copypaste.workspace;

import com.facebook.react.bridge.Arguments;
import com.facebook.react.bridge.Promise;
import com.facebook.react.bridge.ReactApplicationContext;
import com.facebook.react.bridge.ReactContextBaseJavaModule;
import com.facebook.react.bridge.ReactMethod;
import com.facebook.react.bridge.ReadableMap;
import com.facebook.react.bridge.ReadableMapKeySetIterator;
import com.facebook.react.bridge.ReadableType;
import com.facebook.react.bridge.WritableMap;
import java.util.HashMap;
import java.util.Map;

public final class CPHTTPModule extends ReactContextBaseJavaModule {
    private final ApiHTTP http = new ApiHTTP(BuildConfig.CP_API_ORIGIN, BuildConfig.CP_LOCAL_HTTP);
    CPHTTPModule(ReactApplicationContext context) { super(context); }
    @Override public String getName() { return "CPHTTP"; }
    @ReactMethod public void request(String id, String method, String path, ReadableMap input, String body, Promise promise) {
        Map<String, String> headers = new HashMap<>();
        ReadableMapKeySetIterator keys = input.keySetIterator();
        while (keys.hasNextKey()) {
            String key = keys.nextKey();
            if (input.getType(key) != ReadableType.String) { promise.reject("REQUEST_UNCONFIRMED", "The request could not be confirmed."); return; }
            headers.put(key, input.getString(key));
        }
        http.request(id, method, path, headers, body, new ApiHTTP.Result() {
            @Override public void complete(int status, String value) {
                WritableMap result = Arguments.createMap(); result.putInt("status", status); result.putString("body", value); promise.resolve(result);
            }
            @Override public void failed() { promise.reject("REQUEST_UNCONFIRMED", "The request could not be confirmed."); }
        });
    }
    @ReactMethod public void cancel(String id) { http.cancel(id); }
    @Override public void invalidate() { http.close(); super.invalidate(); }
}
