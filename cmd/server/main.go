package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"

	"github.com/ronakg/runner/pkg/lib"
	"github.com/ronakg/runner/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	lib.Debug = true
	lib.RootFSSource = "/tmp/runner/rootfs"
}

func createCredentials(certsDir string) (credentials.TransportCredentials, error) {
	certificate, err := tls.LoadX509KeyPair(
		filepath.Join(certsDir, "server.crt"),
		filepath.Join(certsDir, "server.key"),
	)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(filepath.Join(certsDir, "ca.crt"))
	if err != nil {
		return nil, err
	}

	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	tlsConfig := &tls.Config{
		ClientAuth:   tls.RequireAndVerifyClientCert,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    ca,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
	return credentials.NewTLS(tlsConfig), err
}

func main() {
	// TODO: configuration for server certificates
	// ca.crt, server.crt and server.key are looked up in certsDir
	certsDir := os.Args[1]
	creds, err := createCredentials(certsDir)
	if err != nil {
		log.Fatalf("Failed to set up certificates: %v", err)
	}

	// TODO: Add configurable server port number
	lis, err := net.Listen("tcp", ":9000")

	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	proto.RegisterRunnerServer(grpcServer, newRunnerServer())

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %s", err)
	}
}
