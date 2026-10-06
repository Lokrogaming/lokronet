package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
)

// cmdDashboardWeb: Browser-Dashboard für Server-Checks (nur Loopback).
// Primäre UI bleibt das Terminal-Dashboard (`lokronet dashboard`, bubbletea).
func cmdDashboardWeb(args []string) error {
	fs := flag.NewFlagSet("dashboard-web", flag.ContinueOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "Listen-Adresse (nur Loopback)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(*addr)
	if err != nil {
		return fmt.Errorf("ungültige --addr %q (Format ip:port): %w", *addr, err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("dashboard-web hört aus Sicherheit nur auf Loopback (127.0.0.1/localhost/::1)")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(dashboardWebHTML))
	})
	proxy := func(path string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			data, err := ipcCall("GET", path, nil)
			if err != nil {
				w.WriteHeader(http.StatusBadGateway)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
		}
	}
	mux.HandleFunc("/api/status", proxy("/v1/status"))
	mux.HandleFunc("/api/events", proxy("/v1/events"))
	mux.HandleFunc("/api/chat/sessions", proxy("/v1/chat/sessions"))
	mux.HandleFunc("/api/chat/inbox", proxy("/v1/chat/inbox"))
	fmt.Printf("dashboard-web auf http://%s (nur lokal, STRG+C zum Stoppen) …\n", *addr)
	fmt.Println("hinweis: braucht laufenden `lokronet daemon` – sonst zeigt die Seite dessen Fehler.")
	return http.ListenAndServe(*addr, mux)
}

const dashboardWebHTML = `<!doctype html>
<html lang="de"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>LokroNet Dashboard</title>
<style>body{background:#0d1117;color:#e6edf3;font-family:system-ui,sans-serif;max-width:760px;margin:2rem auto;padding:0 1rem}pre{background:#161b22;padding:1rem;border-radius:8px;overflow-x:auto}.err{color:#f0883e}.ok{color:#3fb950}</style>
</head><body>
<h1>LokroNet Dashboard <span style="font-size:.6em;color:#8b949e">lokal</span></h1>
<p><button onclick="load()">Aktualisieren</button> <span id="state"></span></p>
<pre id="out">lade …</pre>
<h2>Events</h2>
<pre id="events">lade …</pre>
<h2>Chat-Sessions</h2>
<pre id="chat">lade …</pre>
<script>
async function load(){
  const st=document.getElementById('state'), out=document.getElementById('out'), ev=document.getElementById('events'), ch=document.getElementById('chat');
  st.textContent='lade …';
  try{
    let r=await fetch('/api/status'); let j=await r.json();
    if(!r.ok) throw new Error(j.error||r.status);
    out.textContent=JSON.stringify(j,null,2); st.innerHTML='<span class=ok>ok</span>';
  }catch(e){ out.textContent='daemon offline? lokronet daemon starten.\n'+e; st.innerHTML='<span class=err>offline</span>'; }
  try{
    let r=await fetch('/api/events'); let j=await r.json();
    ev.textContent=Array.isArray(j)? j.map(x=>'['+x.time+'] '+x.msg).join('\n')||'(keine Events – lokronet debug on)': JSON.stringify(j,null,2);
  }catch(e){ ev.textContent='events: '+e; }
  try{
    let r=await fetch('/api/chat/sessions'); let j=await r.json();
    ch.textContent=JSON.stringify(j,null,2);
  }catch(e){ ch.textContent='chat: '+e; }
}
load(); setInterval(load,5000);
</script></body></html>`
