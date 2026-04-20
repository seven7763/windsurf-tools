//go:build darwin

package services

// port443_darwin.go — acquires a TLS net.Listener on a privileged port
// (typically 443) without running the whole process as root.
//
// Strategy: a tiny embedded helper binary is launched via
//
//	osascript "do shell script … with administrator privileges"
//
// The helper (running as root) binds TCP 127.0.0.1:443, then passes the
// raw socket file descriptor back to this process over a Unix-domain socket
// using SCM_RIGHTS ancillary data.  This process wraps the received fd with
// tls.NewListener and serves TLS directly — no TCP relay needed.

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"
)

//go:embed assets/porthelper_darwin
var porthelperBin []byte

const (
	porthelperPath = "/tmp/windsurf_tools_porthelper"
	porthelperSock = "/tmp/windsurf_tools_porthelper.sock"
)

// preludeCommands holds shell commands that should be executed as root
// alongside the porthelper invocation (single password prompt).
// Caller sets this before calling listenTLS, via SetPrivilegedPrelude.
var preludeCommands string

// SetPrivilegedPrelude stores shell commands to run as root before the
// porthelper is launched. They share the same osascript prompt.
func SetPrivilegedPrelude(script string) {
	preludeCommands = script
}

// listenTLS returns a TLS listener on 127.0.0.1:<port>.
// For privileged ports (<1024) it uses the embedded helper to acquire the
// socket as root; for unprivileged ports it falls back to plain tls.Listen.
func listenTLS(port int, tlsConfig *tls.Config) (net.Listener, error) {
	if port < 1024 {
		rawLn, err := acquirePrivilegedListener(port)
		if err != nil {
			return nil, err
		}
		return tls.NewListener(rawLn, tlsConfig), nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	return tls.Listen("tcp", addr, tlsConfig)
}

// acquirePrivilegedListener binds the given TCP port as root via osascript
// and returns a net.Listener ready to Accept() incoming connections.
func acquirePrivilegedListener(port int) (net.Listener, error) {
	// 1. Write helper binary to disk.
	if err := os.WriteFile(porthelperPath, porthelperBin, 0755); err != nil {
		return nil, fmt.Errorf("写入 porthelper 失败: %w", err)
	}

	// 2. Prepare Unix-socket rendezvous.
	os.Remove(porthelperSock)
	unixLn, err := net.Listen("unix", porthelperSock)
	if err != nil {
		return nil, fmt.Errorf("创建 Unix socket 失败: %w", err)
	}
	defer unixLn.Close()
	defer os.Remove(porthelperSock)

	// 3. Launch privileged helper asynchronously (may show admin dialog).
	//    If preludeCommands is set, run them (as root) before the helper.
	prelude := ""
	if preludeCommands != "" {
		// Prelude 容错：任何一步失败不阻塞 porthelper 启动
		prelude = "(" + preludeCommands + ") || true; "
	}
	shellCmd := fmt.Sprintf(
		`%schmod +x %s; %s %d %s </dev/null >/dev/null 2>&1`,
		prelude, porthelperPath, porthelperPath, port, porthelperSock,
	)
	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, shellEscapeAppleScript(shellCmd))
	go exec.Command("osascript", "-e", script).Run() //nolint:errcheck
	preludeCommands = ""                             // 单次使用，避免下次启动重复执行

	// 4. Accept helper connection (30 s — user may need to confirm dialog).
	unixLn.(*net.UnixListener).SetDeadline(time.Now().Add(30 * time.Second))
	conn, err := unixLn.Accept()
	if err != nil {
		return nil, fmt.Errorf("等待 porthelper 超时（请确认管理员授权）: %w", err)
	}
	unixConn := conn.(*net.UnixConn)
	defer unixConn.Close()

	// 5. Receive TCP socket fd via SCM_RIGHTS.
	buf := make([]byte, 1)
	oob := make([]byte, syscall.CmsgSpace(4)) // space for one int32
	_, oobn, _, _, err := unixConn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, fmt.Errorf("接收 socket fd 失败: %w", err)
	}
	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) == 0 {
		return nil, fmt.Errorf("解析控制消息失败: %w", err)
	}
	fds, err := syscall.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) == 0 {
		return nil, fmt.Errorf("解析 fd 失败: %w", err)
	}

	// Send ack so helper exits cleanly.
	unixConn.Write([]byte{1}) //nolint:errcheck

	// 6. Wrap raw fd as net.Listener (FileListener dups the fd internally).
	f := os.NewFile(uintptr(fds[0]), fmt.Sprintf("tcp:127.0.0.1:%d", port))
	defer f.Close()
	rawLn, err := net.FileListener(f)
	if err != nil {
		return nil, fmt.Errorf("net.FileListener 失败: %w", err)
	}
	return rawLn, nil
}
