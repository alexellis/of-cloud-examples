package function

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

func Handle(req []byte) string {
	uri := strings.TrimSuffix(string(req), "\n")

	info, err := downloadCertificate(uri)
	if err != nil {
		handleError(http.StatusInternalServerError, err)
		return ""
	}

	infoString := getCertificateDetailsAsString(info)
	fileName, err := saveToS3(infoString, info.certificate.Subject.CommonName)

	if err != nil {
		handleError(http.StatusInternalServerError, err)
		return ""
	}

	return fileName
}

func getCertificateDetailsAsString(info *certificateInfo) string {
	asJSON := os.Getenv("Http_Query")

	if len(asJSON) > 0 && asJSON == "output=json" {
		res := struct {
			Host          string
			Port          string
			Issuer        string
			CommonName    string
			NotBefore     time.Time
			NotAfter      time.Time
			NotAfterUnix  int64
			SANs          []string
			TimeRemaining string
		}{
			info.host,
			info.port,
			info.certificate.Issuer.CommonName,
			info.certificate.Subject.CommonName,
			info.certificate.NotBefore,
			info.certificate.NotAfter,
			info.certificate.NotAfter.Unix(),
			info.certificate.DNSNames,
			humanize.Time(info.certificate.NotAfter),
		}

		b, err := json.Marshal(res)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return string(b)
	}

	return fmt.Sprintf("Host %v\nPort %v\nIssuer %v\nCommonName %v\nNotBefore %v\nNotAfter %v\nNotAfterUnix %v\nSANs %v\nTimeRemaining %v",
		info.host, info.port, info.certificate.Issuer.CommonName, info.certificate.Subject.CommonName,
		info.certificate.NotBefore, info.certificate.NotAfter, info.certificate.NotAfter.Unix(),
		info.certificate.DNSNames, humanize.Time(info.certificate.NotAfter))
}

type certificateInfo struct {
	certificate *x509.Certificate
	host        string
	port        string
}

func downloadCertificate(certificateURI string) (*certificateInfo, error) {
	certificateURI = strings.ToLower(certificateURI)
	if !strings.HasPrefix(certificateURI, "http") {
		certificateURI = "https://" + certificateURI
	}

	u, err := url.Parse(certificateURI)
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}

	address := u.Hostname() + ":443"
	ipConn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("SSL/TLS not enabed on %v\nDial error: %v", u.Hostname(), err)
	}

	defer ipConn.Close()
	conn := tls.Client(ipConn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         u.Hostname(),
	})
	if err = conn.Handshake(); err != nil {
		return nil, fmt.Errorf("Invalid SSL/TLS for %v\nHandshake error: %v", address, err)
	}

	defer conn.Close()
	addr := conn.RemoteAddr()
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, fmt.Errorf("error: %v", err)
	}

	cert := conn.ConnectionState().PeerCertificates[0]
	return &certificateInfo{certificate: cert, host: host, port: port}, nil
}

func handleError(status int, err error) {
	log.Printf("status: %v", status)
	log.Println(err.Error())
}
