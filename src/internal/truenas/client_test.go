package truenas

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeTrueNAS answers the JSON-RPC calls the exporter makes, with a
// notification interleaved before each response to exercise ID matching.
func fakeTrueNAS(t *testing.T, validKey string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/current" {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()
		for {
			var req rpcRequest
			var raw map[string]json.RawMessage
			if err := wsjson.Read(ctx, conn, &raw); err != nil {
				return
			}
			_ = json.Unmarshal(raw["id"], &req.ID)
			_ = json.Unmarshal(raw["method"], &req.Method)
			var params []json.RawMessage
			_ = json.Unmarshal(raw["params"], &params)

			_ = wsjson.Write(ctx, conn, map[string]any{"jsonrpc": "2.0", "method": "collection_update", "params": map[string]any{}})

			var result any
			switch req.Method {
			case "auth.login_with_api_key":
				var key string
				_ = json.Unmarshal(params[0], &key)
				result = key == validKey
			case "pool.query":
				result = []map[string]any{
					{"name": "tank", "status": "ONLINE", "healthy": true, "size": 4000, "allocated": 1500, "free": 2500},
					{"name": "cold", "status": "OFFLINE", "healthy": false, "size": nil, "allocated": nil, "free": nil},
				}
			case "pool.dataset.query":
				result = []map[string]any{
					{"name": "tank", "used": map[string]any{"parsed": 1400, "value": "1.4 KiB"}, "available": map[string]any{"parsed": 2300}},
				}
			default:
				_ = wsjson.Write(ctx, conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "Method not found"}})
				continue
			}
			_ = wsjson.Write(ctx, conn, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		}
	}))
}

func wsURL(srv *httptest.Server) string {
	return "wss" + strings.TrimPrefix(srv.URL, "https") + "/api/current"
}

func fingerprint(srv *httptest.Server) []byte {
	sum := sha256.Sum256(srv.Certificate().Raw)
	return sum[:]
}

func TestDialAndQuery(t *testing.T) {
	srv := fakeTrueNAS(t, "good-key")
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Dial(ctx, Options{URL: wsURL(srv), APIKey: "good-key", TLSFingerprint: fingerprint(srv)})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	pools, err := c.Pools(ctx)
	if err != nil {
		t.Fatalf("Pools: %v", err)
	}
	if len(pools) != 2 || pools[0].Name != "tank" || *pools[0].Size != 4000 || pools[1].Size != nil {
		t.Errorf("unexpected pools: %+v", pools)
	}

	usages, err := c.PoolUsages(ctx, []string{"tank", "cold"})
	if err != nil {
		t.Fatalf("PoolUsages: %v", err)
	}
	if len(usages) != 1 || usages[0] != (PoolUsage{Name: "tank", Used: 1400, Available: 2300}) {
		t.Errorf("unexpected usages: %+v", usages)
	}

	if err := c.Call(ctx, "nope.nope", nil, nil); err == nil || !strings.Contains(err.Error(), "Method not found") {
		t.Errorf("expected RPC error, got %v", err)
	}
}

func TestDialRejectsBadKey(t *testing.T) {
	srv := fakeTrueNAS(t, "good-key")
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Dial(ctx, Options{URL: wsURL(srv), APIKey: "bad-key", TLSFingerprint: fingerprint(srv)})
	if err == nil || !strings.Contains(err.Error(), "API key rejected") {
		t.Fatalf("expected rejection, got %v", err)
	}
}

func TestDialTLSModes(t *testing.T) {
	srv := fakeTrueNAS(t, "k")
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// httptest's cert isn't CA-trusted, so default validation must fail...
	if _, err := Dial(ctx, Options{URL: wsURL(srv), APIKey: "k"}); err == nil {
		t.Error("expected CA validation failure")
	}
	// ...as must a wrong pin...
	wrong := make([]byte, 32)
	if _, err := Dial(ctx, Options{URL: wsURL(srv), APIKey: "k", TLSFingerprint: wrong}); err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Errorf("expected fingerprint mismatch, got %v", err)
	}
	// ...while skip-verify connects.
	c, err := Dial(ctx, Options{URL: wsURL(srv), APIKey: "k", TLSSkipVerify: true})
	if err != nil {
		t.Fatalf("skip-verify Dial: %v", err)
	}
	_ = c.Close()
}
