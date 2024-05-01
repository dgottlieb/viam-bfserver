package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-github/v61/github"
)

func TestJira(t *testing.T) {
	GetOpenBFs()
}

func TestCreateNewTicketFromFailure(t *testing.T) {
	GDebug = true
	ctx := context.Background()
	client := github.NewTokenClient(ctx, githubToken)

	failures, err := GithubRunToFailedTests(ctx, client, 5977123166, 16216407061)
	if err != nil {
		panic(err)
	}

	if len(failures) != 1 {
		panic(fmt.Sprintf("Wrong number of failures: %v", len(failures)))
	}

	CreateNewTicketsFromFailure(failures[0], "", false)
}
