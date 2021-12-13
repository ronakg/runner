package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ronakg/runner/pkg/lib"
	"github.com/ronakg/runner/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type runnerServer struct {
	proto.UnimplementedRunnerServer
	jobs safeJobs
}

func newRunnerServer() *runnerServer {
	return &runnerServer{
		jobs: safeJobs{
			table: make(map[string]lib.Job),
		},
	}
}

func getClientCN(ctx context.Context) (string, error) {
	var cn string
	peer, ok := peer.FromContext(ctx)
	if ok {
		tlsInfo := peer.AuthInfo.(credentials.TLSInfo)
		cn = tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	}
	if cn == "" {
		log.Printf("Client did not provide common name")
		return "", fmt.Errorf("Client did not provide common name")
	}
	return cn, nil
}

func (s *runnerServer) Start(ctx context.Context, req *proto.StartRequest) (*proto.StartResponse, error) {
	cn, err := getClientCN(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, err.Error())
	}

	config := lib.JobConfig{
		Command: req.Command,
		Timeout: time.Duration(req.Timeout) * time.Second,
		Profile: lib.ResProfile(req.Profile),
	}
	log.Printf("Start request: %+v", config)

	j, err := lib.StartJob(config)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, err.Error())
	}
	log.Printf("%s started successfully", j)

	s.jobs.Set(j.ID()+cn, j)

	return &proto.StartResponse{
		JobId: j.ID(),
	}, nil
}

func (s *runnerServer) Stop(ctx context.Context, req *proto.StopRequest) (*proto.StopResponse, error) {
	cn, err := getClientCN(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, err.Error())
	}

	log.Printf("Stop request for job id %s", req.JobId)
	j, ok := s.jobs.Get(req.JobId + cn)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, "Cannot find job %s for %s", req.JobId, cn)
	}

	j.Stop()
	status, ec := j.Status()
	log.Printf("%s stopped successfully. Status: %s (%d)", j, status, ec)

	return &proto.StopResponse{
		Status:   proto.JobStatus(status),
		ExitCode: int32(ec),
	}, nil
}

func (s *runnerServer) Status(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	cn, err := getClientCN(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, err.Error())
	}

	log.Printf("Status request for job id %s", req.JobId)
	j, ok := s.jobs.Get(req.JobId + cn)
	if !ok {
		return nil, status.Errorf(codes.PermissionDenied, "Cannot find job %s for %s", req.JobId, cn)
	}

	status, ec := j.Status()
	log.Printf("Status for %s: %s (%d)", req.JobId, status, ec)

	return &proto.StatusResponse{
		Status:   proto.JobStatus(status),
		ExitCode: int32(ec),
	}, nil
}

func (s *runnerServer) Output(req *proto.OutputRequest, strSrv proto.Runner_OutputServer) error {
	ctx := strSrv.Context()
	cn, err := getClientCN(ctx)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, err.Error())
	}

	log.Printf("Output request from %s for job id %s", cn, req.JobId)
	j, ok := s.jobs.Get(req.JobId + cn)
	if !ok {
		return status.Errorf(codes.PermissionDenied, "Cannot find job %s for %s", req.JobId, cn)
	}
	out, cancel, err := j.Output()
	if err != nil {
		return err
	}
	defer cancel()

	for {
		select {
		case buf, ok := <-out:
			if !ok {
				// out channel closed
				return nil
			}
			err := strSrv.Send(&proto.OutputResponse{
				Buffer: buf.Bytes,
			})
			if err != nil {
				log.Printf("Error sending output to client: %v", err)
				return err
			}
		case <-ctx.Done():
			// client disconnected
			log.Printf("%s disconnected output for %s", cn, req.JobId)
			return nil
		}
	}
}
