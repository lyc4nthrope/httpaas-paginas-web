package web

import "net/http"

// seguridad wraps every route:
//   - Only the hosts the app is served on are accepted, for every method. The app listens
//     on 127.0.0.1 without login, so a foreign Host means DNS rebinding: a malicious site
//     whose name now points to 127.0.0.1.
//   - A POST's Origin, when present, must be exactly this app (CSRF). Browsers always send
//     it on cross-site form posts; without it, Sec-Fetch-Site still marks them.
//   - Request bodies are capped, so a huge upload cannot fill the disk.
//   - Basic headers stop MIME sniffing and framing of the dashboard.
func seguridad(next http.Handler, maxZip int64, hosts []string) http.Handler {
	permitidos := map[string]bool{}
	for _, h := range hosts {
		permitidos[h] = true
	}
	limite := maxZip + 1<<20 // room for the other form fields and multipart headers
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if !permitidos[r.Host] {
			http.Error(w, "host no permitido", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			origen := r.Header.Get("Origin")
			if (origen != "" && origen != "http://"+r.Host) || (origen == "" && r.Header.Get("Sec-Fetch-Site") == "cross-site") {
				http.Error(w, "origen no permitido", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limite)
		}
		next.ServeHTTP(w, r)
	})
}
