package ru.dbcatalog.app;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import android.util.Base64;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.List;

/** Read grants expose one resolved download, never the configuration or key store. */
public final class ExportProvider extends ContentProvider {
    static File verifyDownload(Context context, String path) throws IOException {
        File root = new File(context.getFilesDir(), "downloads").getCanonicalFile();
        File file = new File(path).getCanonicalFile();
        if (!file.getPath().startsWith(root.getPath() + File.separator) || !file.isFile()) {
            throw new FileNotFoundException("Файл отсутствует в загрузках приложения");
        }
        return file;
    }

    static Uri forFile(Context context, File file, String displayName) throws IOException {
        file = verifyDownload(context, file.getPath());
        File root = new File(context.getFilesDir(), "downloads").getCanonicalFile();
        String relative = file.getPath().substring(root.getPath().length() + 1);
        String token = Base64.encodeToString(relative.getBytes(StandardCharsets.UTF_8),
                Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
        return new Uri.Builder().scheme("content").authority(context.getPackageName() + ".exports")
                .appendPath("download").appendPath(token).appendPath(displayName).build();
    }

    private File resolve(Uri uri) throws IOException {
        List<String> parts = uri.getPathSegments();
        if (getContext() == null || !(getContext().getPackageName() + ".exports").equals(uri.getAuthority())
                || parts.size() != 3 || !parts.get(0).equals("download")) {
            throw new FileNotFoundException("Некорректная ссылка на файл");
        }
        try {
            String relative = new String(Base64.decode(parts.get(1),
                    Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING), StandardCharsets.UTF_8);
            if (relative.startsWith("/") || relative.contains("\u0000")) throw new IOException("Некорректный путь");
            return verifyDownload(getContext(), new File(new File(getContext().getFilesDir(), "downloads"), relative).getPath());
        } catch (IllegalArgumentException error) {
            throw new FileNotFoundException("Некорректная ссылка на файл");
        }
    }

    static String mime(String name) {
        int dot = name.lastIndexOf('.');
        String result = dot < 0 ? null : MimeTypeMap.getSingleton().getMimeTypeFromExtension(name.substring(dot + 1).toLowerCase(java.util.Locale.ROOT));
        return result == null ? "application/octet-stream" : result;
    }

    @Override public boolean onCreate() { return true; }

    @Override public String getType(Uri uri) {
        try { return mime(resolve(uri).getName()); }
        catch (IOException error) { return "application/octet-stream"; }
    }

    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] args, String order) {
        try {
            File file = resolve(uri);
            String[] columns = projection == null ? new String[]{OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE} : projection;
            MatrixCursor cursor = new MatrixCursor(columns, 1);
            Object[] values = new Object[columns.length];
            for (int i = 0; i < columns.length; i++) {
                if (OpenableColumns.DISPLAY_NAME.equals(columns[i])) values[i] = uri.getLastPathSegment();
                if (OpenableColumns.SIZE.equals(columns[i])) values[i] = file.length();
            }
            cursor.addRow(values);
            return cursor;
        } catch (IOException error) { return null; }
    }

    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) throw new FileNotFoundException("Доступен только режим чтения");
        try { return ParcelFileDescriptor.open(resolve(uri), ParcelFileDescriptor.MODE_READ_ONLY); }
        catch (IOException error) { throw new FileNotFoundException("Файл недоступен"); }
    }

    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException(); }
    @Override public int update(Uri uri, ContentValues values, String where, String[] args) { throw new UnsupportedOperationException(); }
    @Override public int delete(Uri uri, String where, String[] args) { throw new UnsupportedOperationException(); }
}
