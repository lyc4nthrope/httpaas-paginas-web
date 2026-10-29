package web

import (
	"net/http"
	"net/url"
)

// seguridad wraps every route:
//   - POSTs from another site are refused (CSRF): a browser always sends Origin on a
//     cross-site form POST, and it must match the host the app is served on.
//   - Request bodies are capped, so a huge upload cannot fill the disk.
//   - Basic headers stop MIME sniffing and framing of the dashboard.
func seguridad(next http.Handler, maxZip int64) http.Handler {
	limite := maxZip + 1<<20 // room for the other form fields and multipart headers
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if r.Method == http.MethodPost {
			if origen := r.Header.Get("Origin"); origen != "" {
				u, err := url.Parse(origen)
				if err != nil || u.Host != r.Host {
					http.Error(w, "origen no permitido", http.StatusForbidden)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, limite)
		}
		next.ServeHTTP(w, r)
	})
}
