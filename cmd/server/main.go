package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/benzhi/chao-sheng/internal/audit"
	"github.com/benzhi/chao-sheng/internal/observation"
	"github.com/benzhi/chao-sheng/internal/quality"
	"github.com/benzhi/chao-sheng/internal/repository"
	"github.com/benzhi/chao-sheng/internal/web"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:19081", "监听地址")
	self := flag.Bool("self-check", false, "运行自检后退出")
	flag.Parse()
	if p := os.Getenv("PORT"); p != "" && *addr == "127.0.0.1:19081" {
		*addr = "127.0.0.1:" + p
	}
	repo, e := repository.Open("file:chao-sheng.db?cache=shared")
	if e != nil {
		panic(e)
	}
	defer repo.Close()
	au := audit.New(repo)
	ob := observation.New(repo, au)
	qu := quality.New(repo, au)
	ws := web.New(ob, qu, au, repo)
	if allow := os.Getenv("PUBLISH_ACTORS"); allow != "" {
		ws.PublishActors = map[string]bool{}
		for _, a := range strings.Split(allow, ",") {
			if v := strings.TrimSpace(a); v != "" {
				ws.PublishActors[v] = true
			}
		}
	}
	srv := &http.Server{Addr: *addr, Handler: ws.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if *self {
		go srv.ListenAndServe()
		if e := runCheck(*addr); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		srv.Shutdown(context.Background())
		return
	}
	fmt.Printf("潮声观测质检台监听 %s\n", *addr)
	if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		panic(e)
	}
}
func runCheck(addr string) error {
	base := "http://" + addr
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 20; i++ {
		r, e := client.Get(base + "/health")
		if e == nil {
			r.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	post := func(path string, v interface{}, h map[string]string) ([]byte, error) {
		b, _ := json.Marshal(v)
		req, _ := http.NewRequest("POST", base+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		for k, x := range h {
			req.Header.Set(k, x)
		}
		resp, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		d, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %s", path, d)
		}
		return d, nil
	}
	now := time.Now().UTC()
	runTag := fmt.Sprintf("%d", now.UnixNano())
	var c struct {
		CaseID   string `json:"case_id"`
		Revision int    `json:"revision"`
	}
	d, e := post("/api/v1/cases", map[string]interface{}{"buoy_id": "B-SELF-" + runTag, "region": "黄海", "species_scope": "鲸类", "started_at": now.Add(-time.Hour).Format(time.RFC3339), "ended_at": now.Format(time.RFC3339), "created_by": "observer"}, map[string]string{"X-Request-ID": "self-create-" + runTag})
	if e != nil {
		return e
	}
	json.Unmarshal(d, &c)
	d, e = post("/api/v1/cases/"+c.CaseID+"/evidence", map[string]interface{}{"sensor_id": "S1", "calibration_ref": "CAL-1", "audio_digest": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", "operator": "qc", "calibrated_at": now.Format(time.RFC3339), "sampling_rate": 48000}, map[string]string{"X-Request-ID": "self-evi-" + runTag, "X-Expected-Revision": fmt.Sprint(c.Revision)})
	if e != nil {
		return e
	}
	json.Unmarshal(d, &c)
	_, e = post("/api/v1/cases/"+c.CaseID+"/screen", map[string]string{}, map[string]string{"X-Request-ID": "self-screen-" + runTag, "X-Actor": "qc"})
	if e != nil {
		return e
	}
	json.Unmarshal(d, &c)
	return nil
}
