package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	backupv1 "github.com/diclebolek/Safepoint/api/v1"
)

func main() {
	addr := envOr("ADDR", ":8088")
	scheme := runtime.NewScheme()
	utilruntime.Must(backupv1.AddToScheme(scheme))

	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("kubeconfig: %v", err)
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("client: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_ = pageTmpl.Execute(w, nil)
	})
	mux.HandleFunc("/api/schedules", func(w http.ResponseWriter, r *http.Request) {
		var list backupv1.BackupScheduleList
		if err := c.List(r.Context(), &list); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("/api/restores", func(w http.ResponseWriter, r *http.Request) {
		var list backupv1.BackupRestoreList
		if err := c.List(r.Context(), &list); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("safepoint dashboard on %s", addr)
	_ = context.Background()
	log.Fatal(srv.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var pageTmpl = template.Must(template.New("page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1"/>
  <title>Safepoint Dashboard</title>
  <style>
    :root { --bg:#0f1419; --card:#1a2332; --text:#e7ecf3; --muted:#8b9bb4; --ok:#3dd68c; --bad:#f07178; --run:#ffcc66; }
    body { margin:0; font-family: "Segoe UI", system-ui, sans-serif; background: radial-gradient(1200px 600px at 10% -10%, #1b3a4b, var(--bg)); color: var(--text); }
    header { padding: 28px 32px 8px; }
    h1 { margin:0; font-size: 1.8rem; }
    p { color: var(--muted); }
    main { padding: 16px 32px 48px; display:grid; gap:24px; }
    section { background: var(--card); border-radius: 14px; padding: 18px 20px; }
    table { width:100%; border-collapse: collapse; font-size: .92rem; }
    th, td { text-align:left; padding: 10px 8px; border-bottom: 1px solid #2a3648; }
    th { color: var(--muted); }
    .pill { display:inline-block; padding: 2px 10px; border-radius: 999px; font-size: .78rem; font-weight: 600; }
    .ok { background: rgba(61,214,140,.15); color: var(--ok); }
    .bad { background: rgba(240,113,120,.15); color: var(--bad); }
    .run { background: rgba(255,204,102,.15); color: var(--run); }
    .muted { color: var(--muted); }
    button { background:#2d6cdf; color:white; border:0; border-radius:8px; padding:8px 14px; cursor:pointer; }
  </style>
</head>
<body>
  <header>
    <h1>Safepoint</h1>
    <p>Backup schedules &amp; restores</p>
    <button onclick="refresh()">Refresh</button>
  </header>
  <main>
    <section>
      <h2>BackupSchedules</h2>
      <div id="schedules" class="muted">Loading…</div>
    </section>
    <section>
      <h2>BackupRestores</h2>
      <div id="restores" class="muted">Loading…</div>
    </section>
  </main>
  <script>
    function pill(phase) {
      const p = (phase || 'Unknown');
      let cls = 'run';
      if (p === 'Succeeded') cls = 'ok';
      if (p === 'Failed') cls = 'bad';
      return '<span class="pill '+cls+'">'+p+'</span>';
    }
    async function load(path, el, kind) {
      const box = document.getElementById(el);
      try {
        const res = await fetch(path);
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        const items = data.items || [];
        if (!items.length) { box.innerHTML = '<p class="muted">No '+kind+' found.</p>'; return; }
        let html = '<table><thead><tr><th>Name</th><th>Namespace</th><th>Engine</th><th>Phase</th><th>Details</th></tr></thead><tbody>';
        for (const it of items) {
          const spec = it.spec || {};
          const st = it.status || {};
          const detail = kind === 'schedules'
            ? (st.lastObjectKey || st.message || '')
            : ((spec.objectKey || '') + ' ' + (st.message || ''));
          html += '<tr><td>'+it.metadata.name+'</td><td>'+it.metadata.namespace+'</td><td>'+(spec.engine||'postgres')+'</td><td>'+pill(st.phase)+'</td><td class="muted">'+detail+'</td></tr>';
        }
        html += '</tbody></table>';
        box.innerHTML = html;
      } catch (e) {
        box.innerHTML = '<p class="bad">'+e+'</p>';
      }
    }
    function refresh() {
      load('/api/schedules', 'schedules', 'schedules');
      load('/api/restores', 'restores', 'restores');
    }
    refresh();
    setInterval(refresh, 15000);
  </script>
</body>
</html>`))
