package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-github/v61/github"
	"github.com/viamrobotics/bfserver/util"
)

func TestJira(t *testing.T) {
	GetOpenFlakeyFailureTickets("", "")
}

func TestCreateNewTicketFromFailure(t *testing.T) {
	util.GDebug = true
	ctx := context.Background()
	client := github.NewTokenClient(ctx, githubToken)

	failures, err := GithubRunToFailedTests(ctx, client, "viamrobotics/rdk", // not 100% on the string here
		5977123166, 16216407061)
	if err != nil {
		panic(err)
	}

	if len(failures) != 1 {
		panic(fmt.Sprintf("Wrong number of failures: %v", len(failures)))
	}

	fmt.Println(CreateTicketObjectsFromFailure(failures[0]))
}
