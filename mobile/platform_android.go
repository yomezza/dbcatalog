package mobile

import "os"

func init() {
	// Android 14+ updates system CAs through the Conscrypt APEX. Go's Unix
	// default still searches /system/etc/security/cacerts; include both stores
	// before the first TLS connection. An explicitly selected sslRootCert keeps
	// its existing pgx semantics, and a caller override is left intact.
	if os.Getenv("SSL_CERT_DIR") == "" {
		_ = os.Setenv("SSL_CERT_DIR", "/apex/com.android.conscrypt/cacerts:/system/etc/security/cacerts")
	}
}
