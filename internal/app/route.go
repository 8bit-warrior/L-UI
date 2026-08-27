package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type RouteResult struct {
	Success   bool    `json:"success"`
	Error     string  `json:"error,omitempty"`
	HTTPCode  int     `json:"http_code,omitempty"`
	Outbound  string  `json:"outbound,omitempty"`
	TotalMS   float64 `json:"total_ms,omitempty"`
	ConnectMS float64 `json:"connect_ms,omitempty"`
	TTFBMS    float64 `json:"ttfb_ms,omitempty"`
	RemoteIP  string  `json:"remote_ip,omitempty"`
	AccessLog string  `json:"access_log,omitempty"`
}

func freePort() (int, error) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
func waitPort(port int, timeout time.Duration) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 150*time.Millisecond)
		if e == nil {
			c.Close()
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

var outboundLogRE = regexp.MustCompile(`\[[^\]\n]*?->\s*([^\]\s]+)\]`)

func ParseOutboundFromAccessLog(text string) string {
	xs := outboundLogRE.FindAllStringSubmatch(text, -1)
	if len(xs) == 0 {
		return "未知"
	}
	return xs[len(xs)-1][1]
}

func socks5DialContext(proxy string, connectMetric *time.Duration, remoteIP *string, mu *sync.Mutex) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		started := time.Now()
		d := net.Dialer{Timeout: 8 * time.Second}
		c, e := d.DialContext(ctx, "tcp", proxy)
		if e != nil {
			return nil, e
		}
		fail := func(err error) (net.Conn, error) { c.Close(); return nil, err }
		_ = c.SetDeadline(time.Now().Add(10 * time.Second))
		if _, e = c.Write([]byte{5, 1, 0}); e != nil {
			return fail(e)
		}
		reply := make([]byte, 2)
		if _, e = io.ReadFull(c, reply); e != nil {
			return fail(e)
		}
		if reply[0] != 5 || reply[1] != 0 {
			return fail(errors.New("SOCKS5 authentication negotiation failed"))
		}
		host, portS, e := net.SplitHostPort(address)
		if e != nil {
			return fail(e)
		}
		port, e := strconv.Atoi(portS)
		if e != nil {
			return fail(e)
		}
		req := []byte{5, 1, 0}
		ip := net.ParseIP(host)
		if ip4 := ip.To4(); ip4 != nil {
			req = append(req, 1)
			req = append(req, ip4...)
		} else if ip6 := ip.To16(); ip6 != nil {
			req = append(req, 4)
			req = append(req, ip6...)
		} else {
			hb := []byte(host)
			if len(hb) > 255 {
				return fail(errors.New("SOCKS5 hostname too long"))
			}
			req = append(req, 3, byte(len(hb)))
			req = append(req, hb...)
		}
		pb := make([]byte, 2)
		binary.BigEndian.PutUint16(pb, uint16(port))
		req = append(req, pb...)
		if _, e = c.Write(req); e != nil {
			return fail(e)
		}
		head := make([]byte, 4)
		if _, e = io.ReadFull(c, head); e != nil {
			return fail(e)
		}
		if head[0] != 5 || head[1] != 0 {
			return fail(fmt.Errorf("SOCKS5 CONNECT failed code=%d", head[1]))
		}
		var bound string
		switch head[3] {
		case 1:
			x := make([]byte, 4)
			if _, e = io.ReadFull(c, x); e != nil {
				return fail(e)
			}
			bound = net.IP(x).String()
		case 4:
			x := make([]byte, 16)
			if _, e = io.ReadFull(c, x); e != nil {
				return fail(e)
			}
			bound = net.IP(x).String()
		case 3:
			n := make([]byte, 1)
			if _, e = io.ReadFull(c, n); e != nil {
				return fail(e)
			}
			x := make([]byte, int(n[0]))
			if _, e = io.ReadFull(c, x); e != nil {
				return fail(e)
			}
			bound = string(x)
		default:
			return fail(errors.New("SOCKS5 invalid ATYP"))
		}
		tail := make([]byte, 2)
		if _, e = io.ReadFull(c, tail); e != nil {
			return fail(e)
		}
		_ = c.SetDeadline(time.Time{})
		mu.Lock()
		*connectMetric = time.Since(started)
		if bound != "0.0.0.0" && bound != "::" && bound != "" {
			*remoteIP = bound
		}
		mu.Unlock()
		return c, nil
	}
}

func RealRouteTest(p Paths, st *State, rawURL, inboundTag string, timeout time.Duration) RouteResult {
	if _, e := os.Stat(p.XrayBin); e != nil {
		return RouteResult{Error: "Xray 尚未安装"}
	}
	u, e := url.Parse(rawURL)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return RouteResult{Error: "URL 必须是 http:// 或 https://"}
	}
	port, e := freePort()
	if e != nil {
		return RouteResult{Error: e.Error()}
	}
	td, e := os.MkdirTemp("", "lui-route-test-*")
	if e != nil {
		return RouteResult{Error: e.Error()}
	}
	defer os.RemoveAll(td)
	access := filepath.Join(td, "access.log")
	errlog := filepath.Join(td, "error.log")
	cfg, e := BuildConfig(st)
	if e != nil {
		return RouteResult{Error: e.Error()}
	}
	cfg["log"] = map[string]any{"access": access, "error": errlog, "loglevel": "warning"}
	tag := inboundTag
	if tag == "" {
		tag = "lui-route-test"
	}
	cfg["inbounds"] = []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "socks", "settings": map[string]any{"udp": true}, "sniffing": map[string]any{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": false}, "tag": tag}}
	cfgPath := filepath.Join(td, "config.json")
	if e = AtomicJSON(cfgPath, cfg); e != nil {
		return RouteResult{Error: e.Error()}
	}
	if ok, msg := ValidateXrayConfig(p, cfgPath); !ok {
		return RouteResult{Error: "临时 Xray 配置校验失败: " + msg}
	}
	var procOut bytes.Buffer
	cmd := exec.Command(p.XrayBin, "run", "-config", cfgPath)
	cmd.Stdout = &procOut
	cmd.Stderr = &procOut
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = cmd.Start(); e != nil {
		return RouteResult{Error: e.Error()}
	}
	defer func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			done := make(chan struct{})
			go func() { _, _ = cmd.Process.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
		}
	}()
	if !waitPort(port, 5*time.Second) {
		return RouteResult{Error: "临时 Xray 未能监听测试端口: " + short(procOut.String(), 1500)}
	}
	var connectDur time.Duration
	var remote string
	var mu sync.Mutex
	transport := &http.Transport{Proxy: nil, DialContext: socks5DialContext(fmt.Sprintf("127.0.0.1:%d", port), &connectDur, &remote, &mu), TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true, DisableKeepAlives: true}
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var firstByte time.Time
	trace := &httptrace.ClientTrace{GotFirstResponseByte: func() {
		if firstByte.IsZero() {
			firstByte = time.Now()
		}
	}}
	ctx = httptrace.WithClientTrace(ctx, trace)
	req, _ := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	req.Header.Set("User-Agent", "L-UI-route-test/"+Version)
	started := time.Now()
	resp, e := client.Do(req)
	if e != nil {
		total := time.Since(started)
		return RouteResult{Error: e.Error(), TotalMS: float64(total.Microseconds()) / 1000}
	}
	_, copyErr := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	total := time.Since(started)
	ttfb := time.Duration(0)
	if !firstByte.IsZero() {
		ttfb = firstByte.Sub(started)
	}
	logText := ""
	ob := "未知"
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if d, e := os.ReadFile(access); e == nil {
			logText = string(d)
			ob = ParseOutboundFromAccessLog(logText)
			if ob != "未知" {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	last := ""
	if logText != "" {
		sc := bufio.NewScanner(strings.NewReader(logText))
		for sc.Scan() {
			last = sc.Text()
		}
	}
	mu.Lock()
	cd := connectDur
	rip := remote
	mu.Unlock()
	success := copyErr == nil && resp.StatusCode > 0
	rr := RouteResult{Success: success, HTTPCode: resp.StatusCode, Outbound: ob, TotalMS: float64(total.Microseconds()) / 1000, ConnectMS: float64(cd.Microseconds()) / 1000, TTFBMS: float64(ttfb.Microseconds()) / 1000, RemoteIP: rip, AccessLog: last}
	if copyErr != nil {
		rr.Error = copyErr.Error()
	}
	return rr
}
