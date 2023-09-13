package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/go-github/v53/github"
)

var githubToken string

func init() {
	githubToken = os.Getenv("github_token")
}

func TestGetFailingTestsForRun(t *testing.T) {
	t.Skip()
	ctx := context.Background()
	client := github.NewTokenClient(ctx, githubToken)
	failures, err := GithubRunToFailedTests(ctx, client, 5717936462, 0)
	fmt.Println("Rate:", lastResponse.Rate)
	if err != nil {
		panic(err)
	}

	fmt.Println("Failures:", failures)
}

func TestFindFailingRuns(t *testing.T) {
	t.Skip()
	ctx := context.Background()
	client := github.NewTokenClient(ctx, githubToken)
	// 7 total runs -- 2 failures
	failedRuns, err := FindFailingRuns(ctx, client, "2023-08-01", "2023-08-02")
	fmt.Println("Rate:", lastResponse.Rate)
	if err != nil {
		panic(err)
	}

	fmt.Println("Failures:", failedRuns)
}

func TestRunReport(t *testing.T) {
	ctx := context.Background()
	client := github.NewTokenClient(ctx, githubToken)
	// 7 total runs -- 2 failures
	// failedRuns, err := FindFailingRuns(ctx, client, "2023-08-01", "2023-08-02")

	failedRuns, err := FindFailingRuns(ctx, client, "2023-08-07", "2023-08-08")
	fmt.Println("Rate:", lastResponse.Rate)
	if err != nil {
		panic(err)
	}

	for _, failedRun := range failedRuns {
		// Get logs for run and parse failures
		testFailures, err := GithubRunToFailedTests(ctx, client, failedRun.GetID(), 0)
		if err != nil {
			fmt.Println("Err:", err)
			continue
		}

		if len(testFailures) == 0 {
			// When tests succeed, but the higher level github run fails. For example, failing to
			// build the app image.
			continue
		}

		fmt.Printf(`Failed run: %v
    Time: %v
    RunLink: https://github.com/viamrobotics/rdk/actions/runs/%v
`,
			failedRun.GetID(), failedRun.GetCreatedAt(), failedRun.GetID())
		for _, failure := range testFailures {
			fmt.Printf("\n\t| %v |\n", failure.Variant)
			fmt.Println("\t---------------------------")
			failure.Output.PrettyPrint("\t")
		}
	}

	fmt.Println("Rate:", lastResponse.Rate)
}

func zipFileToJsonDecoder(reader *zip.ReadCloser) *json.Decoder {
	// _foo_.zip file will contain a single log file.
	if cnt := len(reader.File); cnt != 1 {
		panic(fmt.Sprintf("Too many files. Cnt: %v", cnt))
	}

	zipFile := reader.File[0]
	fileReader, err := zipFile.Open()
	if err != nil {
		panic(err)
	}
	// Dan: I do not believe the `fileReader` needs to be closed.

	return json.NewDecoder(fileReader)
}

func TestCaptureContext(t *testing.T) {
	t.Skip()

	var outputs *Output
	var contextTestFile *zip.ReadCloser
	var err error
	ctx := context.Background()

	for _, filename := range []string{
		"./testdata/failure_context_test_logs.json.zip",
		"./testdata/timeout_context_test_logs.json.zip",
		"./testdata/datarace_context_test_logs.json.zip",
	} {
		contextTestFile, err = zip.OpenReader(filename)
		if err != nil {
			panic(err)
		}
		defer contextTestFile.Close()

		outputs, _ = parseFailures(ctx, zipFileToJsonDecoder(contextTestFile))
		outputs.PrettyPrint("\t")
	}
}
