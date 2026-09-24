// Package truenas is a minimal client for the TrueNAS JSON-RPC 2.0 websocket
// API (TrueNAS 25.04+), covering only the calls this exporter needs.
package truenas

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Options configures how Dial connects.
type Options struct {
	URL            string
	APIKey         string
	TLSFingerprint []byte // SHA-256 of the leaf certificate; bypasses CA validation when set
	TLSSkipVerify  bool
}

// Client is a single authenticated websocket session. It is not safe for
// concurrent use; callers open one per collection.
type Client struct {
	conn   *websocket.Conn
	nextID int
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

type rpcResponse struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Reason string `json:"reason"`
	} `json:"data"`
}

func (e *rpcError) Error() string {
	if e.Data.Reason != "" {
		return fmt.Sprintf("%s (%d): %s", e.Message, e.Code, e.Data.Reason)
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

// Dial connects to TrueNAS and authenticates with the API key.
func Dial(ctx context.Context, opts Options) (*Client, error) {
	conn, _, err := websocket.Dial(ctx, opts.URL, &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig(opts)}},
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", opts.URL, err)
	}
	// pool.query responses include full vdev topology; the 32KiB default is too small.
	conn.SetReadLimit(16 << 20)

	c := &Client{conn: conn}
	var ok bool
	if err := c.Call(ctx, "auth.login_with_api_key", []any{opts.APIKey}, &ok); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("authenticating: %w", err)
	}
	if !ok {
		_ = c.Close()
		return nil, fmt.Errorf("authenticating: API key rejected")
	}
	return c, nil
}

func tlsConfig(opts Options) *tls.Config {
	if opts.TLSFingerprint != nil {
		want := opts.TLSFingerprint
		return &tls.Config{
			// Chain/hostname validation is replaced, not skipped: the
			// callback below rejects any certificate but the pinned one.
			InsecureSkipVerify: true, //nolint:gosec
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				if len(rawCerts) == 0 {
					return fmt.Errorf("server presented no certificate")
				}
				got := sha256.Sum256(rawCerts[0])
				if !bytes.Equal(got[:], want) {
					return fmt.Errorf("certificate fingerprint mismatch: got %s", hex.EncodeToString(got[:]))
				}
				return nil
			},
		}
	}
	return &tls.Config{InsecureSkipVerify: opts.TLSSkipVerify} //nolint:gosec
}

// Call invokes a method and decodes its result into out (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params []any, out any) error {
	c.nextID++
	id := c.nextID
	if params == nil {
		params = []any{}
	}
	if err := wsjson.Write(ctx, c.conn, rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return fmt.Errorf("%s: sending request: %w", method, err)
	}
	for {
		var resp rpcResponse
		if err := wsjson.Read(ctx, c.conn, &resp); err != nil {
			return fmt.Errorf("%s: reading response: %w", method, err)
		}
		// Skip server-initiated notifications (no id) and anything stale.
		if resp.ID == nil || *resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("%s: %w", method, resp.Error)
		}
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("%s: decoding result: %w", method, err)
		}
		return nil
	}
}

// Close ends the session.
func (c *Client) Close() error {
	return c.conn.Close(websocket.StatusNormalClosure, "")
}
