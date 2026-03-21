package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	caDirName = ".windsurf-tools"
	caSubDir  = "ca"
)

func caCertDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, caDirName, caSubDir)
}

func caCertPath() string  { return filepath.Join(caCertDir(), "ca.pem") }
func caKeyPath() string   { return filepath.Join(caCertDir(), "ca.key") }
func hostCertPath() string { return filepath.Join(caCertDir(), "host.pem") }
func hostKeyPath() string  { return filepath.Join(caCertDir(), "host.key") }

// EnsureCA generates a self-signed CA if not already present,
// then generates a host certificate for the target domain.
func EnsureCA(targetDomain string) (*tls.Certificate, error) {
	dir := caCertDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建 CA 目录失败: %w", err)
	}

	// 1. Ensure CA exists
	if _, err := os.Stat(caCertPath()); os.IsNotExist(err) {
		if err := generateCA(); err != nil {
			return nil, fmt.Errorf("生成 CA 失败: %w", err)
		}
	}

	// 2. Load CA
	caCert, caKey, err := loadCA()
	if err != nil {
		return nil, fmt.Errorf("加载 CA 失败: %w", err)
	}

	// 3. Generate host cert (always regenerate to ensure domain match)
	if err := generateHostCert(caCert, caKey, targetDomain); err != nil {
		return nil, fmt.Errorf("生成域名证书失败: %w", err)
	}

	// 4. Load host cert
	hostTLS, err := tls.LoadX509KeyPair(hostCertPath(), hostKeyPath())
	if err != nil {
		return nil, fmt.Errorf("加载域名证书失败: %w", err)
	}

	return &hostTLS, nil
}

func generateCA() error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Windsurf Tools CA",
			Organization: []string{"Windsurf Tools"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}

	// Write cert
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(caCertPath(), certPEM, 0644); err != nil {
		return err
	}

	// Write key
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return os.WriteFile(caKeyPath(), keyPEM, 0600)
}

func loadCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(caCertPath())
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("无法解码 CA PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err := os.ReadFile(caKeyPath())
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("无法解码 CA key PEM")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

func generateHostCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey, domain string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	// ★ 证书 SAN 包含所有劫持域名，否则 server.codeium.com 流量会 TLS 校验失败
	dnsNames := []string{domain}
	for _, t := range hostsTargets {
		if t != domain {
			dnsNames = append(dnsNames, t)
		}
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"Windsurf Tools"},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(2 * 365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames:    dnsNames,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(hostCertPath(), certPEM, 0644); err != nil {
		return err
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return os.WriteFile(hostKeyPath(), keyPEM, 0600)
}

// InstallCA installs the CA certificate to the system trust store.
func InstallCA() error {
	certPath := caCertPath()
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("CA 证书不存在，请先生成")
	}

	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("certutil", "-addstore", "Root", certPath)
		hideWindow(cmd)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("安装 CA 失败: %s\n%s", err, string(output))
		}
	case "darwin":
		cmd := exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot",
			"-k", "/Library/Keychains/System.keychain", certPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("安装 CA 失败: %s\n%s", err, string(output))
		}
	default:
		// Linux: copy to /usr/local/share/ca-certificates/ and run update-ca-certificates
		dstDir := "/usr/local/share/ca-certificates"
		dstFile := filepath.Join(dstDir, "windsurf-tools-ca.crt")
		data, err := os.ReadFile(certPath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstFile, data, 0644); err != nil {
			return fmt.Errorf("复制 CA 到系统目录失败(需要 sudo): %w", err)
		}
		cmd := exec.Command("update-ca-certificates")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("update-ca-certificates 失败: %s\n%s", err, string(output))
		}
	}
	return nil
}

// UninstallCA removes the CA certificate from the system trust store.
func UninstallCA() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("certutil", "-delstore", "Root", "Windsurf Tools CA")
		hideWindow(cmd)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("卸载 CA 失败: %s\n%s", err, string(output))
		}
	case "darwin":
		certPath := caCertPath()
		cmd := exec.Command("security", "remove-trusted-cert",
			"-d", certPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("卸载 CA 失败: %s\n%s", err, string(output))
		}
	default:
		dstFile := "/usr/local/share/ca-certificates/windsurf-tools-ca.crt"
		_ = os.Remove(dstFile)
		cmd := exec.Command("update-ca-certificates", "--fresh")
		_, _ = cmd.CombinedOutput()
	}
	return nil
}

var (
	caInstalledCache     bool
	caInstalledCacheTime time.Time
)

// IsCAInstalled checks if the CA is installed in the system trust store.
// Result is cached for 30 seconds to avoid repeated certutil invocations.
func IsCAInstalled() bool {
	if time.Since(caInstalledCacheTime) < 30*time.Second {
		return caInstalledCache
	}

	result := isCAInstalledUncached()
	caInstalledCache = result
	caInstalledCacheTime = time.Now()
	return result
}

func isCAInstalledUncached() bool {
	if _, err := os.Stat(caCertPath()); os.IsNotExist(err) {
		return false
	}
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("certutil", "-verifystore", "Root", "Windsurf Tools CA")
		hideWindow(cmd)
		err := cmd.Run()
		return err == nil
	default:
		return true // simplified check for non-Windows
	}
}

// InvalidateCACache forces the next IsCAInstalled() call to re-check.
func InvalidateCACache() {
	caInstalledCacheTime = time.Time{}
}

// GetCACertPath returns the CA certificate file path.
func GetCACertPath() string {
	return caCertPath()
}
