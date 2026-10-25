package ru.dbcatalog.smoke;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Intent;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.WebView;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;
import org.json.JSONArray;
import org.json.JSONObject;
import org.json.JSONTokener;

/** Separate, same-certificate instrumentation APK. Never packaged in the app.
 * Uses ONLY the disposable compose PostgreSQL and a guarded synthetic schema. */
public final class NativeSmoke extends Instrumentation {
    private Bundle arguments;
    private WebView web;
    @Override public void onCreate(Bundle args) { super.onCreate(args); arguments = args; start(); }
    @Override public void onStart() {
        Bundle result = new Bundle();
        try {
            String schema = arguments.getString("schema", "");
            check(schema.matches("android_smoke_[a-z0-9_]{1,40}"), "Unsafe test schema");
            Activity activity = startActivitySync(new Intent().setClassName("ru.dbcatalog.app", "ru.dbcatalog.app.MainActivity")
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            long deadline = SystemClock.elapsedRealtime() + 180000;
            while (web == null && SystemClock.elapsedRealtime() < deadline) {
                runOnMainSync(() -> web = findWeb(activity.getWindow().getDecorView()));
                SystemClock.sleep(250);
            }
            check(web != null, "Native host did not expose its WebView");
            while (!Boolean.TRUE.equals(eval("Boolean(window.go && window.go.main && window.go.main.App)"))
                    && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(250);
            check(Boolean.TRUE.equals(eval("document.documentElement.dataset.platform === 'android'")), "Android UI bridge missing");
            JSONObject diagnostics = (JSONObject) call("GetDiagnostics");
            check(diagnostics.getString("platform").startsWith("android/"), "Go is not running on Android");
            check(diagnostics.getString("credentialBackend").equals("Android Keystore"), "Wrong native credential store");
            if ("resume".equals(arguments.getString("phase"))) {
                // App startup itself reconnects using the encrypted stored password.
                JSONObject saved = (JSONObject) call("GetConnection");
                check(saved.getBoolean("hasPassword"), "Encrypted password not recovered after process restart");
                check(saved.getJSONObject("connection").getString("schema").equals(schema), "Wrong resumed schema");
                JSONObject state = (JSONObject) call("ResumeSession");
                check(!state.isNull("user"), "Saved session token did not resume");
                check(((JSONObject) call("GetCatalog")).getJSONArray("servers").length() == 1, "Catalog not preserved");
            } else {
                JSONObject connection = new JSONObject().put("host", "10.0.2.2").put("port", 55432)
                        .put("database", "catalogdb_test").put("username", "catalogdb").put("schema", schema)
                        .put("sslMode", "disable").put("sslRootCert", "");
                call("Connect", connection, "catalogdb-test-only", false, true);
                call("Register", "tablet-smoke", "android-smoke-only-1234", true);
                call("LoginWithSession", "tablet-smoke", "android-smoke-only-1234", 24);
                eval("window.__smokeEvents=0;window.runtime.EventsOn('catalogdb:change',()=>window.__smokeEvents++);true");
                JSONObject server = (JSONObject) call("SaveObject", new JSONObject().put("kind", "server").put("name", "TABLET-DEMO"));
                check(!server.getString("id").isEmpty(), "Go/PostgreSQL did not save the server");
                deadline = SystemClock.elapsedRealtime() + 15000;
                while (!Boolean.TRUE.equals(eval("window.__smokeEvents>0")) && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(250);
                check(Boolean.TRUE.equals(eval("window.__smokeEvents>0")), "Native LISTEN/NOTIFY event missing");
                JSONObject download = (JSONObject) call("ExportToDownloads");
                check(!download.getString("id").isEmpty(), "Native Excel export failed");
                check(((JSONArray) call("ListDownloads")).length() > 0, "Local downloads history missing");
                check(((JSONObject) call("GetDiagnostics")).getString("credentialState").equals("available"), "Keystore save failed");
            }
            result.putString("stream", "PASS: Android WebView, Go, PostgreSQL, Keystore, export and session phase=" + arguments.getString("phase", "create") + "\n");
            finish(Activity.RESULT_OK, result);
        } catch (Throwable error) {
            result.putString("stream", "FAIL: " + error.toString() + "\n");
            finish(Activity.RESULT_CANCELED, result);
        }
    }
    private static WebView findWeb(View view) {
        if (view instanceof WebView) return (WebView) view;
        if (view instanceof ViewGroup) for (int i=0; i<((ViewGroup)view).getChildCount(); i++) {
            WebView found = findWeb(((ViewGroup)view).getChildAt(i)); if (found != null) return found;
        }
        return null;
    }
    private Object eval(String script) throws Exception {
        CountDownLatch ready = new CountDownLatch(1);
        AtomicReference<String> output = new AtomicReference<>();
        runOnMainSync(() -> web.evaluateJavascript(script, value -> { output.set(value); ready.countDown(); }));
        check(ready.await(120, TimeUnit.SECONDS), "WebView callback timed out");
        return new JSONTokener(output.get()).nextValue();
    }
    private Object call(String method, Object... values) throws Exception {
        JSONArray args = new JSONArray(); for (Object value : values) args.put(value);
        eval("window.__nativeSmoke=null;(async()=>{try{window.__nativeSmoke={value:await window.go.main.App["
                + JSONObject.quote(method) + "](..." + args + ")}}catch(e){window.__nativeSmoke={error:String(e)}}})();null");
        long deadline = SystemClock.elapsedRealtime() + 180000;
        while (SystemClock.elapsedRealtime() < deadline) {
            Object raw = eval("JSON.stringify(window.__nativeSmoke)");
            Object decoded = new JSONTokener((String) raw).nextValue();
            if (decoded instanceof JSONObject) {
                JSONObject response = (JSONObject) decoded;
                check(!response.has("error"), method + ": " + response.optString("error"));
                return response.opt("value");
            }
            SystemClock.sleep(250);
        }
        throw new Exception(method + " timed out");
    }
    private static void check(boolean condition, String message) throws Exception { if (!condition) throw new Exception(message); }
}
