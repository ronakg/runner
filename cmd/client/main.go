package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func getClientConn() *grpc.ClientConn {
	creds, err := createCredentials(certsDir)
	if err != nil {
		log.Fatalf("Failed to set up certificates: %v", err)
	}

	conn, err := grpc.Dial(fmt.Sprintf(":%s", port), grpc.WithTransportCredentials(creds))
	if err != nil {
		log.Fatalf("grpc server error: %v", err)
	}
	return conn
}

func createCredentials(certsDir string) (credentials.TransportCredentials, error) {
	certificate, err := tls.LoadX509KeyPair(
		filepath.Join(certsDir, "client.crt"),
		filepath.Join(certsDir, "client.key"),
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
		Certificates: []tls.Certificate{certificate},
		RootCAs:      ca,
	}
	return credentials.NewTLS(tlsConfig), err
}

var certsDir string
var port string

func main() {
	cobra.EnableCommandSorting = false
	cmd := &cobra.Command{
		Use:   "client",
		Short: "Runner client",
	}
	cmd.PersistentFlags().StringVar(&certsDir, "certs", "", "Path to the certs directory containing ca.crt, client.crt and client.key")
	cmd.PersistentFlags().StringVarP(&port, "port", "", "9000", "Server port number")
	_ = cmd.MarkFlagRequired("certs")
	cmd.Flags().SortFlags = false

	cmd.AddCommand(startCmd())
	cmd.AddCommand(stopCmd())
	cmd.AddCommand(statusCmd())
	cmd.AddCommand(outputCmd())

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
