package main

import (
	"github.com/spf13/cobra"
)

func startCmd() *cobra.Command {
	var timeout int32
	var profile string
	cmd := &cobra.Command{
		Use:     "start \"command to run\"",
		Short:   "start a new job",
		Example: "client --certs ... start --timeout 1 cp /path/to/source /path/to/destination",
		Args:    cobra.MinimumNArgs(1),
		Run:     startHandler(&timeout, &profile),
	}
	cmd.Flags().Int32VarP(&timeout, "timeout", "t", 0, "[Optional] Timeout in seconds (default no timeout)")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "[Optional] Resource profile for the job")
	cmd.Flags().SortFlags = false

	return cmd
}

func stopCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "stop --id <job_id>",
		Short:   "Stop a job",
		Example: "client stop --id <job_id>",
		Run:     stopHandler(&id),
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Job ID")
	return cmd
}

func statusCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "status --id <job_id>",
		Short:   "Fetch status of a job",
		Example: "client status --id <job_id>",
		Run:     statusHandler(&id),
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Job ID")
	return cmd
}

func outputCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:     "output --id <job_id>",
		Short:   "Print output from a job",
		Example: "client output --id <job_id>",
		Run:     outputHandler(&id),
	}
	cmd.Flags().StringVarP(&id, "id", "i", "", "Job ID")
	return cmd
}
