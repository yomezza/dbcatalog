package ru.dbcatalog.app;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.Intent;
import android.content.res.Configuration;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.view.View;
import android.view.WindowManager;
import android.view.WindowInsets;
import android.webkit.JavascriptInterface;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.widget.FrameLayout;
import android.widget.ProgressBar;
import android.widget.TextView;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.security.cert.Certificate;
import java.security.cert.CertificateFactory;
import java.util.Collection;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.RejectedExecutionException;
import org.json.JSONArray;
import org.json.JSONObject;
import mobile.Client;
import mobile.EventSink;
import mobile.Mobile;

/** Thin Android host; shared Go code owns authentication and all catalogue writes. */
public final class MainActivity extends Activity {
    private static final int PICK_UPLOAD = 100;
    private static final int SAVE_DOCUMENT = 101;
    private static final int PICK_CERTIFICATE = 102;
    private static final int MAX_REQUEST = 48 * 1024 * 1024;
    private static final int MAX_CERTIFICATE = 2 * 1024 * 1024;
    private final Handler main = new Handler(Looper.getMainLooper());
    private final ExecutorService workers = Executors.newFixedThreadPool(2);
    private final Object clientLock = new Object();
    private volatile Client client;
    private volatile boolean closing;
    private WebView webView;
    private FrameLayout root;
    private ValueCallback<Uri[]> uploadCallback;
    private PendingSave pendingSave;
    private String pendingCertificate;

    private static final class PendingSave {
        final String id;
        final File file;
        final byte[] bytes;
        PendingSave(String id, File file, byte[] bytes) { this.id = id; this.file = file; this.bytes = bytes; }
    }

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        root = new FrameLayout(this);
        root.setBackgroundColor(Color.rgb(243, 246, 250));
        setContentView(root);
        root.setOnApplyWindowInsetsListener((view, insets) -> {
            if (Build.VERSION.SDK_INT >= 30) {
                android.graphics.Insets bars = insets.getInsets(WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
                android.graphics.Insets keyboard = insets.getInsets(WindowInsets.Type.ime());
                view.setPadding(bars.left, bars.top, bars.right, Math.max(bars.bottom, keyboard.bottom));
                return WindowInsets.CONSUMED;
            }
            view.setPadding(insets.getSystemWindowInsetLeft(), insets.getSystemWindowInsetTop(),
                    insets.getSystemWindowInsetRight(), insets.getSystemWindowInsetBottom());
            return insets.consumeSystemWindowInsets();
        });
        root.requestApplyInsets();
        ProgressBar progress = new ProgressBar(this);
        FrameLayout.LayoutParams indicator = new FrameLayout.LayoutParams(64, 64, android.view.Gravity.CENTER);
        root.addView(progress, indicator);
        try { createWebView(); }
        catch (RuntimeException error) {
            startupError("Не удалось открыть Android System WebView. Включите или обновите системный WebView и повторите запуск.");
            return;
        }
        workers.execute(() -> {
            try {
                Client next = Mobile.newClient(new File(getFilesDir(), "connection.json").getAbsolutePath(),
                        new SecureCredentials(this), new EventSink() {
                            @Override public void emit(String schema, String type) {
                                try {
                                    JSONObject data = new JSONObject().put("schema", schema).put("type", type);
                                    JSONObject event = new JSONObject().put("name", "catalogdb:change").put("data", data);
                                    javascript("window.__catalogdbAndroidEvent && window.__catalogdbAndroidEvent(" + event + ");");
                                } catch (Exception ignored) { /* Only fixed strings enter event envelopes. */ }
                            }
                        });
                synchronized (clientLock) {
                    if (closing) { next.close(); return; }
                    client = next;
                }
                main.post(() -> {
                    if (closing || webView == null) return;
                    root.removeView(progress);
                    root.addView(webView, new FrameLayout.LayoutParams(-1, -1));
                    webView.loadUrl(LocalAssetsClient.ORIGIN + "/index.html");
                });
            } catch (Throwable error) {
                main.post(() -> startupError("Не удалось запустить Android-компонент CatalogDB. Проверьте версию Android и архитектуру установленного APK."));
            }
        });
    }

    private void createWebView() {
        WebView.setWebContentsDebuggingEnabled(false);
        webView = new WebView(this);
        webView.setBackgroundColor(Color.rgb(243, 246, 250));
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(true);
        settings.setAllowFileAccessFromFileURLs(false);
        settings.setAllowUniversalAccessFromFileURLs(false);
        settings.setMixedContentMode(WebSettings.MIXED_CONTENT_NEVER_ALLOW);
        settings.setSupportMultipleWindows(false);
        settings.setJavaScriptCanOpenWindowsAutomatically(false);
        settings.setMediaPlaybackRequiresUserGesture(true);
        webView.setWebViewClient(new LocalAssetsClient(this, view -> {
            if (view != webView) return;
            root.removeView(view);
            view.removeJavascriptInterface("CatalogDBAndroid");
            view.destroy();
            webView = null;
            startupError("Системный WebView завершил работу. Обновите Android System WebView и перезапустите CatalogDB.");
        }));
        webView.setWebChromeClient(new WebChromeClient() {
            @Override public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback, FileChooserParams params) {
                if (pendingSave != null || pendingCertificate != null) { callback.onReceiveValue(null); return true; }
                if (uploadCallback != null) uploadCallback.onReceiveValue(null);
                uploadCallback = callback;
                try {
                    Intent intent = params.createIntent();
                    intent.addCategory(Intent.CATEGORY_OPENABLE);
                    startActivityForResult(intent, PICK_UPLOAD);
                } catch (ActivityNotFoundException error) {
                    uploadCallback = null;
                    callback.onReceiveValue(null);
                    showMessage("Не найден системный выбор файлов. Установите файловый менеджер с поддержкой документов.");
                }
                return true;
            }
        });
        webView.addJavascriptInterface(new Bridge(), "CatalogDBAndroid");
    }

    private final class Bridge {
        @JavascriptInterface public void postMessage(String message) {
            if (closing || message == null || message.length() > MAX_REQUEST) return;
            try { workers.execute(() -> dispatch(message)); }
            catch (RejectedExecutionException ignored) { /* Activity has just closed. */ }
        }
    }

    private void dispatch(String message) {
        String id = null;
        try {
            JSONObject request = new JSONObject(message);
            id = request.getString("id");
            String method = request.getString("method");
            JSONArray args = request.getJSONArray("args");
            if (!id.matches("[A-Za-z0-9_-]{1,80}") || !method.matches("[A-Z][A-Za-z0-9]{0,63}")) return;
            if (client == null) { failure(id, "Приложение ещё запускается"); return; }
            final String requestId = id;
            switch (method) {
                case "IsSetupMode": success(id, false); return;
                case "Quit": main.post(this::finish); success(id, null); return;
                case "OpenDownload": openDownload(id, args.getString(0), false); return;
                case "ShowDownload": openDownload(id, args.getString(0), true); return;
                case "SaveDownloadAs": {
                    JSONObject info = new JSONObject(client.downloadInfo(args.getString(0)));
                    File file = ExportProvider.verifyDownload(this, info.getString("path"));
                    main.post(() -> createDocument(requestId, info.optString("name", file.getName()),
                            ExportProvider.mime(file.getName()), file, null));
                    return;
                }
                case "SaveDiagnosticsReport": {
                    byte[] bytes = client.diagnosticsReport().getBytes(StandardCharsets.UTF_8);
                    main.post(() -> createDocument(requestId, "CatalogDB-diagnostics.json", "application/json", null, bytes));
                    return;
                }
                case "PickRootCertificate": main.post(() -> pickCertificate(requestId)); return;
                case "ConnectToServer": connectToServer(id, args.getString(0), args.getString(1)); return;
                default:
                    JSONObject response = new JSONObject(client.call(method, args.toString()));
                    response.put("id", id);
                    deliver(response);
            }
        } catch (Exception error) {
            if (id != null && id.matches("[A-Za-z0-9_-]{1,80}")) {
                // Go service errors are intentionally public; filesystem/Java exceptions are not.
                failure(id, "Не удалось выполнить действие Android. Проверьте файл, доступ и состояние подключения.");
            }
        }
    }

    private void deliver(JSONObject response) {
        javascript("window.__catalogdbAndroidReceive && window.__catalogdbAndroidReceive(" + response + ");");
    }

    private void success(String id, Object result) {
        try { deliver(new JSONObject().put("id", id).put("result", result == null ? JSONObject.NULL : result)); }
        catch (Exception ignored) { }
    }

    private void failure(String id, String message) {
        try { deliver(new JSONObject().put("id", id).put("error", message)); }
        catch (Exception ignored) { }
    }

    private void javascript(String script) {
        main.post(() -> { if (!closing && webView != null) webView.evaluateJavascript(script, null); });
    }

    private boolean pickerBusy() { return uploadCallback != null || pendingSave != null || pendingCertificate != null; }

    private boolean externalDocument(Uri uri) {
        if (uri == null || !"content".equals(uri.getScheme())) return false;
        String authority = uri.getAuthority();
        if (authority == null || authority.isEmpty()) return false;
        // Android can prefix content authorities with a work-profile user ID.
        int at = authority.lastIndexOf('@');
        if (at >= 0) authority = authority.substring(at + 1);
        return !(getPackageName() + ".exports").equalsIgnoreCase(authority);
    }

    private void createDocument(String id, String name, String mime, File file, byte[] bytes) {
        if (closing) return;
        if (pickerBusy()) { failure(id, "Завершите текущее действие с файлом"); return; }
        pendingSave = new PendingSave(id, file, bytes);
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE)
                .setType(mime).putExtra(Intent.EXTRA_TITLE, name);
        try { startActivityForResult(intent, SAVE_DOCUMENT); }
        catch (ActivityNotFoundException error) {
            pendingSave = null;
            failure(id, "Не найден системный диалог сохранения файлов");
        }
    }

    private void pickCertificate(String id) {
        if (closing) return;
        if (pickerBusy()) { failure(id, "Завершите текущее действие с файлом"); return; }
        pendingCertificate = id;
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*");
        try { startActivityForResult(intent, PICK_CERTIFICATE); }
        catch (ActivityNotFoundException error) {
            pendingCertificate = null;
            failure(id, "Не найден системный выбор сертификата");
        }
    }

    @Override protected void onActivityResult(int request, int result, Intent data) {
        super.onActivityResult(request, result, data);
        if (request == PICK_UPLOAD) {
            ValueCallback<Uri[]> callback = uploadCallback;
            uploadCallback = null;
            if (callback != null) {
                Uri[] selected = WebChromeClient.FileChooserParams.parseResult(result, data);
                if (selected != null) {
                    for (Uri uri : selected) {
                        if (!externalDocument(uri)) {
                            callback.onReceiveValue(null);
                            showMessage("Выберите файл через внешний системный провайдер документов");
                            return;
                        }
                    }
                }
                try { callback.onReceiveValue(selected); }
                catch (SecurityException error) { showMessage("Нет доступа к выбранному документу. Выберите файл повторно."); }
            }
            return;
        }
        if (request == SAVE_DOCUMENT) {
            PendingSave pending = pendingSave;
            pendingSave = null;
            if (pending == null) return;
            if (result != RESULT_OK || data == null || data.getData() == null) { success(pending.id, false); return; }
            Uri target = data.getData();
            if (!externalDocument(target)) { failure(pending.id, "Выберите внешний документ для сохранения"); return; }
            workers.execute(() -> saveDocument(pending, target));
            return;
        }
        if (request == PICK_CERTIFICATE) {
            String id = pendingCertificate;
            pendingCertificate = null;
            if (id == null) return;
            if (result != RESULT_OK || data == null || data.getData() == null) { success(id, ""); return; }
            Uri source = data.getData();
            if (!externalDocument(source)) { failure(id, "Выберите сертификат через внешний провайдер документов"); return; }
            workers.execute(() -> importCertificate(id, source));
        }
    }

    private void saveDocument(PendingSave pending, Uri target) {
        try {
            if (!externalDocument(target)) throw new Exception("Ожидался внешний системный документ");
            if (pending.file != null) ExportProvider.verifyDownload(this, pending.file.getPath());
            try (InputStream input = pending.file == null ? new ByteArrayInputStream(pending.bytes) : new FileInputStream(pending.file);
                    OutputStream output = getContentResolver().openOutputStream(target, "wt")) {
                if (output == null) throw new Exception("Документ недоступен");
                copy(input, output);
                output.flush();
            }
            success(pending.id, true);
        } catch (Exception error) { failure(pending.id, "Не удалось сохранить файл в выбранный документ"); }
    }

    private void importCertificate(String id, Uri source) {
        File target = null;
        try {
            if (!externalDocument(source)) throw new Exception("Ожидался внешний системный документ");
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            try (InputStream input = getContentResolver().openInputStream(source)) {
                if (input == null) throw new Exception("Сертификат недоступен");
                byte[] buffer = new byte[8192];
                int n;
                while ((n = input.read(buffer)) != -1) {
                    if (bytes.size() + n > MAX_CERTIFICATE) throw new Exception("Размер сертификата превышает 2 МиБ");
                    bytes.write(buffer, 0, n);
                }
            }
            Collection<? extends Certificate> certificates = CertificateFactory.getInstance("X.509")
                    .generateCertificates(new ByteArrayInputStream(bytes.toByteArray()));
            if (certificates.isEmpty()) throw new Exception("Нет сертификатов X.509");
            File folder = new File(getFilesDir(), "certificates");
            if (!folder.isDirectory() && !folder.mkdirs()) throw new Exception("Недоступна папка сертификатов");
            target = File.createTempFile("root-", ".pem", folder);
            try (OutputStream output = new FileOutputStream(target)) {
                for (Certificate certificate : certificates) {
                    String encoded = java.util.Base64.getMimeEncoder(64, new byte[]{'\n'}).encodeToString(certificate.getEncoded());
                    output.write(("-----BEGIN CERTIFICATE-----\n" + encoded + "\n-----END CERTIFICATE-----\n").getBytes(StandardCharsets.US_ASCII));
                }
            }
            success(id, target.getAbsolutePath());
        } catch (Exception error) {
            if (target != null) target.delete();
            failure(id, "Не удалось импортировать сертификат. Выберите X.509 PEM/DER размером до 2 МиБ.");
        }
    }

    private void openDownload(String id, String download, boolean share) throws Exception {
        JSONObject info = new JSONObject(client.downloadInfo(download));
        File file = ExportProvider.verifyDownload(this, info.getString("path"));
        String name = info.optString("name", file.getName());
        String lower = name.toLowerCase(Locale.ROOT);
        if (!share && (lower.endsWith(".sql") || lower.endsWith(".ps1"))) {
            failure(id, "Сохраните скрипт и откройте его в выбранном редакторе");
            return;
        }
        Uri uri = ExportProvider.forFile(this, file, name);
        String mime = ExportProvider.mime(name);
        Intent intent = share ? new Intent(Intent.ACTION_SEND).setType(mime).putExtra(Intent.EXTRA_STREAM, uri)
                : new Intent(Intent.ACTION_VIEW).setDataAndType(uri, mime);
        intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        intent.setClipData(ClipData.newRawUri(name, uri));
        if (!share && intent.resolveActivity(getPackageManager()) == null) {
            failure(id, "Не найдено приложение для этого типа файла");
            return;
        }
        main.post(() -> {
            if (closing) return;
            try { startActivity(Intent.createChooser(intent, share ? "Поделиться файлом" : "Открыть файл")); success(id, null); }
            catch (ActivityNotFoundException error) { failure(id, "Не найдено приложение для этого типа файла"); }
        });
    }

    private void connectToServer(String id, String kind, String object) throws Exception {
        JSONObject target = new JSONObject(client.connectionInfo(kind, object));
        String host = target.getString("host");
        String method = target.getString("method");
        boolean ssh = "SSH".equals(method);
        if (!ssh && !"RDP".equals(method)) { failure(id, "Укажите подключение SSH или RDP в карточке сервера"); return; }
        int port = ssh ? 22 : 3389;
        String address = host.contains(":") ? "[" + host + "]" : host;
        Uri uri = ssh ? Uri.parse("ssh://" + address + ":22")
                : Uri.parse("rdp://full%20address=s:" + Uri.encode(address + ":3389"));
        Intent intent = new Intent(Intent.ACTION_VIEW, uri);
        if (intent.resolveActivity(getPackageManager()) == null) {
            failure(id, "Не найден " + method + "-клиент с поддержкой подключения по ссылке. Установите подходящий клиент.");
            return;
        }
        try (Socket socket = new Socket()) { socket.connect(new InetSocketAddress(host, port), 4000); }
        catch (Exception error) { failure(id, "Сервер недоступен на порту " + port + ". Проверьте сеть, VPN и службу."); return; }
        main.post(() -> {
            if (closing) return;
            try { startActivity(intent); success(id, null); }
            catch (ActivityNotFoundException error) { failure(id, "Не удалось запустить " + method + "-клиент"); }
        });
    }

    private static void copy(InputStream input, OutputStream output) throws Exception {
        byte[] buffer = new byte[32768];
        int size;
        while ((size = input.read(buffer)) != -1) output.write(buffer, 0, size);
    }

    private void startupError(String message) {
        if (closing) return;
        root.removeAllViews();
        TextView error = new TextView(this);
        error.setText(message);
        error.setTextColor(Color.rgb(33, 53, 71));
        error.setTextSize(18);
        error.setPadding(32, 32, 32, 32);
        root.addView(error);
    }

    private void showMessage(String message) {
        if (!closing) new AlertDialog.Builder(this).setMessage(message).setPositiveButton("OK", null).show();
    }

    @Override public void onBackPressed() {
        if (webView == null) { finish(); return; }
        webView.evaluateJavascript("Boolean(document.querySelector('[role=dialog], [role=menu]'))", value -> {
            if ("true".equals(value)) {
                javascript("document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));");
            } else if (!closing) {
                new AlertDialog.Builder(this).setMessage("Закрыть CatalogDB?")
                        .setNegativeButton("Остаться", null).setPositiveButton("Закрыть", (dialog, which) -> finish()).show();
            }
        });
    }

    @Override protected void onResume() {
        super.onResume();
        if (webView != null) {
            webView.onResume();
            javascript("document.dispatchEvent(new Event('visibilitychange'));");
        }
    }

    @Override protected void onPause() {
        if (webView != null) webView.onPause();
        super.onPause();
    }

    @Override public void onConfigurationChanged(Configuration configuration) {
        super.onConfigurationChanged(configuration);
        if (root != null) root.requestApplyInsets();
    }

    @Override protected void onDestroy() {
        final Client current;
        synchronized (clientLock) {
            closing = true;
            current = client;
            client = null;
        }
        if (uploadCallback != null) { uploadCallback.onReceiveValue(null); uploadCallback = null; }
        if (webView != null) {
            webView.removeJavascriptInterface("CatalogDBAndroid");
            webView.stopLoading();
            webView.destroy();
            webView = null;
        }
        workers.execute(() -> { if (current != null) current.close(); });
        workers.shutdown();
        super.onDestroy();
    }
}
