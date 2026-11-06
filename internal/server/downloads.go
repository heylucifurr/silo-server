package server

import (
	"net/http"
	"os"
	"path/filepath"
)

// DownloadsDir is the host directory APK downloads are served from — a
// mounted volume, never the embedded build or the git repo, so shipping a
// new build is copying a file into this directory, not a rebuild. Empty
// disables the /downloads/ route (every request 404s).
var DownloadsDir string

// apkDownloadFiles maps the public /downloads/<slug> path suffix to the
// fixed filename ops copies into DownloadsDir. Keys are matched exactly as
// written here (case-sensitive), so both /downloads/TV and /downloads/tv
// are listed for the same build.
var apkDownloadFiles = map[string]string{
	"TV":      "weflix-tv.apk",
	"tv":      "weflix-tv.apk",
	"android": "weflix.apk",
}

// DownloadsHandler serves the APK named by apkDownloadFiles for the request
// path's slug, reading it from DownloadsDir, and forces an immediate
// download with the right content type. The slug is only ever used as a
// map key, never joined into a filesystem path, so it cannot escape
// DownloadsDir.
func DownloadsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		filename, ok := apkDownloadFiles[slug]
		if !ok || DownloadsDir == "" {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(DownloadsDir, filename)
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		http.ServeContent(w, r, filename, info.ModTime(), f)
	})
}
