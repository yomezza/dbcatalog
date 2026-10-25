package ru.dbcatalog.app;

import android.content.Context;
import android.net.Uri;
import android.webkit.SslErrorHandler;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.RenderProcessGoneDetail;
import android.net.http.SslError;
import java.io.ByteArrayInputStream;
import java.io.IOException;
import java.util.HashMap;
import java.util.Locale;
import java.util.Map;
import java.util.function.Consumer;

/** The WebView origin is local assets only; all PostgreSQL traffic belongs to Go. */
final class LocalAssetsClient extends WebViewClient {
    static final String ORIGIN = "https://appassets.androidplatform.net";
    private final Context context;
    private final Consumer<WebView> rendererFailure;

    LocalAssetsClient(Context context, Consumer<WebView> rendererFailure) {
        this.context = context.getApplicationContext();
        this.rendererFailure = rendererFailure;
    }

    @Override public boolean onRenderProcessGone(WebView view, RenderProcessGoneDetail detail) {
        rendererFailure.accept(view);
        return true;
    }

    static boolean trusted(Uri uri) {
        return "https".equals(uri.getScheme()) && "appassets.androidplatform.net".equals(uri.getHost())
                && (uri.getPort() == -1 || uri.getPort() == 443) && uri.getUserInfo() == null;
    }

    @Override public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
        return !trusted(request.getUrl());
    }

    @Override public boolean shouldOverrideUrlLoading(WebView view, String url) {
        return !trusted(Uri.parse(url));
    }

    @Override public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
        handler.cancel();
    }

    @Override public WebResourceResponse shouldInterceptRequest(WebView view, WebResourceRequest request) {
        if (!trusted(request.getUrl()) || !"GET".equals(request.getMethod())) return rejected();
        return asset(request.getUrl());
    }

    @Override public WebResourceResponse shouldInterceptRequest(WebView view, String url) {
        Uri uri = Uri.parse(url);
        return trusted(uri) ? asset(uri) : rejected();
    }

    private WebResourceResponse asset(Uri uri) {
        String path = uri.getPath();
        if (path == null || path.isEmpty() || path.equals("/")) path = "/index.html";
        if (!path.startsWith("/") || path.contains("\\") || path.contains("\u0000")) return rejected();
        for (String part : path.split("/")) if (part.equals("..") || part.equals(".")) return rejected();
        try {
            WebResourceResponse response = new WebResourceResponse(mime(path), "UTF-8", context.getAssets().open("web" + path));
            Map<String, String> headers = new HashMap<>();
            headers.put("X-Content-Type-Options", "nosniff");
            headers.put("Cache-Control", "no-store");
            headers.put("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'self'");
            response.setResponseHeaders(headers);
            return response;
        } catch (IOException error) { return rejected(); }
    }

    private static String mime(String path) {
        String name = path.toLowerCase(Locale.ROOT);
        if (name.endsWith(".html")) return "text/html";
        if (name.endsWith(".js")) return "application/javascript";
        if (name.endsWith(".css")) return "text/css";
        if (name.endsWith(".svg")) return "image/svg+xml";
        if (name.endsWith(".png")) return "image/png";
        if (name.endsWith(".jpg") || name.endsWith(".jpeg")) return "image/jpeg";
        if (name.endsWith(".woff2")) return "font/woff2";
        if (name.endsWith(".woff")) return "font/woff";
        if (name.endsWith(".json")) return "application/json";
        return "application/octet-stream";
    }

    private static WebResourceResponse rejected() {
        return new WebResourceResponse("text/plain", "UTF-8", 403, "Forbidden", new HashMap<>(), new ByteArrayInputStream(new byte[0]));
    }
}
