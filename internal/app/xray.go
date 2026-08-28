package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Release struct {
	TagName     string         `json:"tag_name"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt string         `json:"published_at"`
	CreatedAt   string         `json:"created_at"`
	Assets      []ReleaseAsset `json:"assets"`
}
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	Size               int64  `json:"size"`
}

func command(name string, args ...string) (int, string) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	}
	return -1, string(out) + err.Error()
}
func commandTimeout(timeout time.Duration, name string, args ...string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return -1, "timeout: " + string(out)
	}
	if err == nil {
		return 0, string(out)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), string(out)
	}
	return -1, string(out) + err.Error()
}
func existsCmd(n string) bool { _, e := exec.LookPath(n); return e == nil }

func ValidateXrayConfig(p Paths, path string) (bool, string) {
	if _, err := os.Stat(p.XrayBin); err != nil {
		data, e := os.ReadFile(path)
		if e != nil {
			return false, e.Error()
		}
		var x any
		if e = json.Unmarshal(data, &x); e != nil {
			return false, e.Error()
		}
		return true, "仅完成 JSON 结构校验（Xray 尚未安装）"
	}
	failures := []string{}
	seen := map[string]bool{}
	for _, args := range [][]string{{"run", "-test", "-config", path}, {"-test", "-config", path}} {
		code, out := commandTimeout(20*time.Second, p.XrayBin, args...)
		out = strings.TrimSpace(out)
		if code == 0 {
			return true, out
		}
		if out != "" && !seen[out] {
			seen[out] = true
			if len(out) > 4096 {
				out = out[:4096] + "...（输出已截断）"
			}
			failures = append(failures, out)
		}
	}
	if len(failures) > 0 {
		return false, "Xray -test 失败:\n" + strings.Join(failures, "\n---\n")
	}
	return false, "Xray -test 失败（Xray 未返回错误信息）"
}

func githubGetJSON(path string, out any) error {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+XrayRepo+path, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "L-UI/"+Version)
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}
func ListReleases() ([]Release, error) {
	var rs []Release
	if err := githubGetJSON("/releases?per_page=30", &rs); err != nil {
		return nil, err
	}
	out := rs[:0]
	for _, r := range rs {
		if !r.Draft {
			out = append(out, r)
		}
	}
	return out, nil
}
func LatestXrayVersion() (string, error) {
	rs, err := ListReleases()
	if err != nil {
		return "", err
	}
	if len(rs) == 0 {
		return "", errors.New("GitHub 未返回 Xray release")
	}
	sort.Slice(rs, func(i, j int) bool {
		ai := rs[i].PublishedAt
		if ai == "" {
			ai = rs[i].CreatedAt
		}
		aj := rs[j].PublishedAt
		if aj == "" {
			aj = rs[j].CreatedAt
		}
		return ai > aj
	})
	return rs[0].TagName, nil
}
func ReleaseForVersion(v string) (*Release, error) {
	v = NormalizeVersion(v)
	var r Release
	err := githubGetJSON("/releases/tags/"+v, &r)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
func PlatformAssetCandidates() []string {
	switch runtime.GOARCH {
	case "amd64":
		return []string{"Xray-linux-64.zip"}
	case "arm64":
		return []string{"Xray-linux-arm64-v8a.zip", "Xray-linux-arm64.zip"}
	case "arm":
		return []string{"Xray-linux-arm32-v7a.zip", "Xray-linux-arm32-v7.zip"}
	case "386":
		return []string{"Xray-linux-32.zip"}
	default:
		return []string{"Xray-linux-" + runtime.GOARCH + ".zip"}
	}
}
func selectAsset(r *Release) *ReleaseAsset {
	for _, want := range PlatformAssetCandidates() {
		for idx := range r.Assets {
			if r.Assets[idx].Name == want {
				return &r.Assets[idx]
			}
		}
	}
	return nil
}
func downloadTo(url, path string, max int64) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "L-UI/"+Version)
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if max > 0 && resp.ContentLength > max {
		return errors.New("下载文件过大")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := io.Reader(resp.Body)
	if max > 0 {
		r = io.LimitReader(resp.Body, max+1)
	}
	n, err := io.Copy(f, r)
	if err != nil {
		return err
	}
	if max > 0 && n > max {
		return errors.New("下载文件超过限制")
	}
	return f.Sync()
}

func InstallXray(p Paths, st *State, version string) (bool, string) {
	version = NormalizeVersion(version)
	rel, err := ReleaseForVersion(version)
	if err != nil {
		return false, "不存在此版本或无法读取 release: " + err.Error()
	}
	asset := selectAsset(rel)
	if asset == nil {
		return false, "版本存在，但没有当前架构 " + runtime.GOARCH + " 对应的 Linux 安装包"
	}
	if err = EnsureDirs(p); err != nil {
		return false, err.Error()
	}
	td, err := os.MkdirTemp("", "lui-xray-*")
	if err != nil {
		return false, err.Error()
	}
	defer os.RemoveAll(td)
	zpath := filepath.Join(td, "xray.zip")
	if err = downloadTo(asset.BrowserDownloadURL, zpath, 256<<20); err != nil {
		return false, "下载失败: " + err.Error()
	}
	if strings.HasPrefix(asset.Digest, "sha256:") {
		data, _ := os.ReadFile(zpath)
		h := sha256.Sum256(data)
		actual := hex.EncodeToString(h[:])
		want := strings.TrimPrefix(asset.Digest, "sha256:")
		if !strings.EqualFold(actual, want) {
			return false, "Xray release SHA256 校验失败"
		}
	}
	zr, err := zip.OpenReader(zpath)
	if err != nil {
		return false, err.Error()
	}
	defer zr.Close()
	candidate := filepath.Join(td, "xray")
	found := false
	for _, f := range zr.File {
		if filepath.Base(f.Name) != "xray" {
			continue
		}
		rc, e := f.Open()
		if e != nil {
			return false, e.Error()
		}
		out, e := os.OpenFile(candidate, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if e != nil {
			rc.Close()
			return false, e.Error()
		}
		_, e = io.Copy(out, io.LimitReader(rc, 256<<20))
		rc.Close()
		out.Close()
		if e != nil {
			return false, e.Error()
		}
		found = true
		break
	}
	if !found {
		return false, "release 压缩包内未找到 xray"
	}
	code, out := commandTimeout(10*time.Second, candidate, "version")
	if code != 0 {
		return false, "下载的 Xray 无法执行: " + out
	}
	first := strings.Split(strings.TrimSpace(out), "\n")[0]
	if !strings.Contains(first, strings.TrimPrefix(version, "v")) {
		return false, fmt.Sprintf("内核版本校验失败：期望 %s，实际 %q", version, first)
	}
	old := p.XrayBin + ".old"
	hadOld := false
	if _, e := os.Stat(p.XrayBin); e == nil {
		hadOld = true
		_ = copyFile(p.XrayBin, old, 0755)
	}
	staged := p.XrayBin + ".new"
	if err = copyFile(candidate, staged, 0755); err != nil {
		return false, err.Error()
	}
	if err = os.Rename(staged, p.XrayBin); err != nil {
		return false, err.Error()
	}
	rollback := func(reason string) (bool, string) {
		if hadOld {
			_ = os.Rename(old, p.XrayBin)
		} else {
			_ = os.Remove(p.XrayBin)
		}
		_, _ = RestartService(p, true)
		return false, reason + "，已回滚旧内核"
	}
	if err = InstallServiceFiles(p); err != nil {
		return rollback("安装服务文件失败: " + err.Error())
	}
	ok, msg := ValidateXrayConfig(p, p.Config)
	if !ok {
		return rollback("新内核无法通过当前配置校验: " + msg)
	}
	oldMeta := DeepCopy(st.Xray)
	st.Xray = map[string]any{"version": version, "installed_at": NowISO()}
	if err = AtomicJSON(p.State, st); err != nil {
		st.Xray = oldMeta
		return rollback("保存版本状态失败")
	}
	if os.Geteuid() == 0 {
		if ok, _ := RestartService(p, true); !ok {
			st.Xray = oldMeta
			_ = AtomicJSON(p.State, st)
			return rollback("新内核安装后服务启动失败")
		}
	}
	_ = os.Remove(old)
	return true, "已安装 " + version
}
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if e := out.Sync(); err == nil {
		err = e
	}
	if e := out.Close(); err == nil {
		err = e
	}
	return err
}

func initKind() string {
	if existsCmd("systemctl") && fileExists("/run/systemd/system") {
		return "systemd"
	}
	if existsCmd("rc-service") && existsCmd("rc-update") {
		return "openrc"
	}
	if fileExists("/etc/init.d") {
		return "sysv"
	}
	return "direct"
}
func fileExists(p string) bool { _, e := os.Stat(p); return e == nil }
func InstallServiceFiles(p Paths) error {
	if os.Geteuid() != 0 {
		return nil
	}
	switch initKind() {
	case "systemd":
		unit := fmt.Sprintf(`[Unit]
Description=L-UI managed Xray
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s run -config %s
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`, p.XrayBin, p.Config)
		if err := os.WriteFile("/etc/systemd/system/l-ui-xray.service", []byte(unit), 0644); err != nil {
			return err
		}
		_, _ = command("systemctl", "daemon-reload")
		return nil
	case "openrc":
		script := fmt.Sprintf(`#!/sbin/openrc-run
name="L-UI Xray"
description="L-UI managed Xray"
command="%s"
command_args="run -config %s"
command_background="yes"
pidfile="/run/l-ui-xray.pid"
output_log="%s"
error_log="%s"
depend() { need net; after firewall; }
`, p.XrayBin, p.Config, p.AccessLog, p.ErrorLog)
		return os.WriteFile("/etc/init.d/l-ui-xray", []byte(script), 0755)
	case "sysv":
		script := fmt.Sprintf(`#!/bin/sh
### BEGIN INIT INFO
# Provides:          l-ui-xray
# Required-Start:    $network
# Required-Stop:     $network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
### END INIT INFO
BIN='%s'
CFG='%s'
PID='%s'
case "$1" in
 start) "$BIN" run -config "$CFG" >/dev/null 2>&1 & echo $! > "$PID" ;;
 stop) [ -f "$PID" ] && kill "$(cat "$PID")" 2>/dev/null; rm -f "$PID" ;;
 restart) "$0" stop; sleep 1; "$0" start ;;
 status) [ -f "$PID" ] && kill -0 "$(cat "$PID")" 2>/dev/null ;;
 *) echo "usage: $0 {start|stop|restart|status}"; exit 2 ;;
esac
`, p.XrayBin, p.Config, p.PIDFile)
		return os.WriteFile("/etc/init.d/l-ui-xray", []byte(script), 0755)
	}
	return nil
}

func ServiceStatus(p Paths) string {
	switch initKind() {
	case "systemd":
		c, o := command("systemctl", "is-active", "l-ui-xray.service")
		if c == 0 {
			return strings.TrimSpace(o)
		}
		return "inactive"
	case "openrc":
		c, _ := command("rc-service", "l-ui-xray", "status")
		if c == 0 {
			return "active"
		}
		return "inactive"
	case "sysv":
		c, _ := command("/etc/init.d/l-ui-xray", "status")
		if c == 0 {
			return "active"
		}
		return "inactive"
	default:
		pid, err := readPID(p.PIDFile)
		if err == nil && processAlive(pid) {
			return "active"
		}
		return "inactive"
	}
}
func ServiceAction(p Paths, action string) (bool, string) {
	if os.Geteuid() != 0 {
		return false, "服务管理需要 root"
	}
	if err := InstallServiceFiles(p); err != nil {
		return false, err.Error()
	}
	switch initKind() {
	case "systemd":
		args := []string{action}
		if action == "enable" || action == "disable" {
			args = append(args, "--now")
		}
		args = append(args, "l-ui-xray.service")
		c, o := command("systemctl", args...)
		return c == 0, o
	case "openrc":
		switch action {
		case "enable":
			c, o := command("rc-update", "add", "l-ui-xray", "default")
			if c == 0 {
				_, _ = command("rc-service", "l-ui-xray", "start")
			}
			return c == 0, o
		case "disable":
			_, _ = command("rc-service", "l-ui-xray", "stop")
			c, o := command("rc-update", "del", "l-ui-xray", "default")
			return c == 0, o
		default:
			c, o := command("rc-service", "l-ui-xray", action)
			return c == 0, o
		}
	case "sysv":
		if action == "enable" {
			if existsCmd("update-rc.d") {
				c, o := command("update-rc.d", "l-ui-xray", "defaults")
				return c == 0, o
			}
			return true, "init script 已安装"
		}
		if action == "disable" {
			if existsCmd("update-rc.d") {
				c, o := command("update-rc.d", "-f", "l-ui-xray", "remove")
				return c == 0, o
			}
			return true, "请由发行版 init 工具管理自启"
		}
		c, o := command("/etc/init.d/l-ui-xray", action)
		return c == 0, o
	default:
		return directServiceAction(p, action)
	}
}
func RestartService(p Paths, bestEffort bool) (bool, string) {
	ok, msg := ServiceAction(p, "restart")
	if !ok && !bestEffort {
		return false, msg
	}
	return ok, msg
}
func directServiceAction(p Paths, action string) (bool, string) {
	switch action {
	case "start":
		if _, e := os.Stat(p.XrayBin); e != nil {
			return false, "Xray 未安装"
		}
		if ServiceStatus(p) == "active" {
			return true, "already running"
		}
		log, _ := os.OpenFile(p.ErrorLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		cmd := exec.Command(p.XrayBin, "run", "-config", p.Config)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if e := cmd.Start(); e != nil {
			return false, e.Error()
		}
		_ = os.WriteFile(p.PIDFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
		_ = cmd.Process.Release()
		return true, "started"
	case "stop":
		pid, e := readPID(p.PIDFile)
		if e != nil {
			return true, "not running"
		}
		if proc, e := os.FindProcess(pid); e == nil {
			_ = proc.Signal(syscall.SIGTERM)
		}
		_ = os.Remove(p.PIDFile)
		return true, "stopped"
	case "restart":
		_, _ = directServiceAction(p, "stop")
		time.Sleep(200 * time.Millisecond)
		return directServiceAction(p, "start")
	case "enable", "disable":
		return false, "当前系统没有可识别的 init 管理器，无法配置开机自启"
	}
	return false, "未知服务操作"
}
func readPID(path string) (int, error) {
	d, e := os.ReadFile(path)
	if e != nil {
		return 0, e
	}
	return strconv.Atoi(strings.TrimSpace(string(d)))
}
func processAlive(pid int) bool {
	p, e := os.FindProcess(pid)
	return e == nil && p.Signal(syscall.Signal(0)) == nil
}

func XrayVersion(p Paths) string {
	if _, e := os.Stat(p.XrayBin); e != nil {
		return "未安装"
	}
	_, o := commandTimeout(5*time.Second, p.XrayBin, "version")
	return strings.TrimSpace(o)
}

func UninstallRuntime(p Paths) error {
	switch initKind() {
	case "systemd":
		_, _ = command("systemctl", "disable", "--now", "l-ui-xray.service")
		_ = os.Remove("/etc/systemd/system/l-ui-xray.service")
		_, _ = command("systemctl", "daemon-reload")
	case "openrc":
		_, _ = command("rc-service", "l-ui-xray", "stop")
		_, _ = command("rc-update", "del", "l-ui-xray", "default")
		_ = os.Remove("/etc/init.d/l-ui-xray")
	case "sysv":
		_, _ = command("/etc/init.d/l-ui-xray", "stop")
		_ = os.Remove("/etc/init.d/l-ui-xray")
	default:
		_, _ = directServiceAction(p, "stop")
	}
	_ = os.Remove(p.XrayBin)
	_ = os.Remove(p.State)
	_ = os.Remove(p.Config)
	if exe, e := os.Executable(); e == nil && filepath.Base(exe) == "l-ui" {
		_ = os.Remove(exe)
	}
	return nil
}
