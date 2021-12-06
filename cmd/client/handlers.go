package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/ronakg/runner/pkg/proto"
	"github.com/spf13/cobra"
)

func startHandler(timeout *int32, profile *string) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, args []string) {
		conn := getClientConn()
		defer conn.Close()

		client := proto.NewRunnerClient(conn)
		resp, err := client.Start(context.Background(), &proto.StartRequest{
			Command: strings.Join(args, " "),
			Timeout: *timeout,
			Profile: *profile,
		})
		if err != nil {
			log.Fatalf("Failed to start '%s': %v", args, err)
		}
		fmt.Printf("%s\n", resp.JobId)
	}
}

func stopHandler(id *string) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, _ []string) {
		conn := getClientConn()
		defer conn.Close()

		client := proto.NewRunnerClient(conn)
		resp, err := client.Stop(context.Background(), &proto.StopRequest{
			JobId: *id,
		})
		if err != nil {
			log.Fatalf("Failed to stop the job %s: %v", *id, err)
		}
		fmt.Printf("%s", resp.Status)
		if resp.Status != proto.JobStatus_RUNNING {
			fmt.Printf(" (%d)", resp.ExitCode)
		}
		fmt.Print("\n")
	}
}

func statusHandler(id *string) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, _ []string) {
		conn := getClientConn()
		defer conn.Close()

		client := proto.NewRunnerClient(conn)
		resp, err := client.Status(context.Background(), &proto.StatusRequest{
			JobId: *id,
		})
		if err != nil {
			log.Fatalf("Failed to get status of the job %s: %v", *id, err)
		}
		fmt.Printf("%s", resp.Status)
		if resp.Status != proto.JobStatus_RUNNING {
			fmt.Printf(" (%d)", resp.ExitCode)
		}
		fmt.Print("\n")
	}
}

func outputHandler(id *string) func(*cobra.Command, []string) {
	return func(_ *cobra.Command, _ []string) {
		conn := getClientConn()
		defer conn.Close()

		client := proto.NewRunnerClient(conn)
		stream, err := client.Output(context.Background(), &proto.OutputRequest{
			JobId: *id,
		})
		if err != nil {
			log.Fatalf("Failed to fetch output of the job %s: %v", *id, err)
		}

		for {
			resp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					log.Fatalf("Server error: %v", err)
				}
				return
			}
			fmt.Printf("%s", resp.Buffer)
		}
	}
}
