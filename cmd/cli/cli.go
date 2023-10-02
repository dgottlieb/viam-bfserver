package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v53/github"
	"github.com/viamrobotics/bfserver/service"
	"github.com/viamrobotics/bfserver/util"
)

func main() {
	fmt.Println("Run started:", time.Now())
	if len(os.Args) == 1 {
		fmt.Println("Usage:\n\tbfserver discover\n\tbfserver analyze")
		return
	}

	switch os.Args[1] {
	case "analyze":
		analyze()
	case "discover":
		discover()
	case "list":
		list()
	case "test":
		test()
	default:
		fmt.Printf("Unknown command: `%v`\n", os.Args[1])
		fmt.Println("Usage:\n\tbfserver discover\n\tbfserver analyze")
		return
	}

	fmt.Println("Github Rate:", service.GithubRate())
}

func test() {
	args := util.ParseProgramArgs()
	ticket := "RSDK-5192"

	jiraClient := args.GetJiraClient()
	links, _, err := jiraClient.Issue.GetRemoteLinks(ticket)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Links: %#v\n", links)
}

func discover() {
	arg := util.ParseProgramArgs()
	fmt.Println("Debug?", util.GDebug)

	var startDate, endDate string

	emptyArgs := make([]string, 0)
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-") == false {
			emptyArgs = append(emptyArgs, arg)
		}
	}

	switch len(emptyArgs) {
	case 1:
		fmt.Println("Usage: bfserver discover <start-date> <end-date?>")
		return
	case 2:
		startDate = emptyArgs[1]
	case 3:
		startDate = emptyArgs[1]
		endDate = emptyArgs[2]
	default:
		fmt.Println("Usage: bfserver discover <start-date> <end-date?>")
		return
	}

	ctx := context.Background()
	client := arg.GetGithubClient()
	runs, err := service.FindFailingRuns(ctx, client, startDate, endDate)
	if err != nil {
		panic(err)
	}

	// For deduping.
	openIssues := service.GetOpenFlakeyFailureTickets(arg.JiraUsername, arg.JiraToken)

	for _, run := range runs {
		if inSeenCache(*run.ID) {
			if arg.HandRun == false {
				fmt.Println("Run in cache. Skipping:", *run.ID, "Date:", *run.RunStartedAt, "Link:", *run.HTMLURL)
				continue
			} else {
				fmt.Println("Run in cache. Handrunning:", *run.ID, "Date:", *run.RunStartedAt, "Link:", *run.HTMLURL)
			}
		}
		fmt.Println("New run:", *run.ID, "Date:", *run.RunStartedAt, "Link:", *run.HTMLURL)
		i1 := service.NewIndenter()

		failures, err := service.GithubRunToFailedTests(ctx, client, *run.ID, int64(0))
		if err != nil {
			panic(err)

		}
		fmt.Println("Num testing job failures:", len(failures))

		for _, failure := range failures {
			fmt.Printf("Failure: %v Link: %v\n", failure.Variant, failure.GithubLink)
			i2 := service.NewIndenter()
			tickets := service.CreateTicketObjectsFromFailure(failure)
			fmt.Printf("NumTickets: %v\n", len(tickets))
			if arg.FileTickets {
				err = service.PushTickets(tickets, openIssues, arg.JiraUsername, arg.JiraToken)
				if err != nil {
					panic(err)
				}
			}

			for idx, ticket := range tickets {
				if arg.FileTickets {
					fmt.Println("Ticket:", ticket.Key)
				} else {
					fmt.Printf("Unfiled ticket #%d\n", idx+1)
				}
				i3 := service.NewIndenter()
				fmt.Println("Summary:", ticket.Fields.Summary)
				fmt.Printf("Description:\n%v\n\n", ticket.Fields.Description)
				i3.Close()
			}
			i2.Close()
		}

		if arg.FileTickets {
			writeSeenCache(*run.ID)
		}
		i1.Close()
	}
}

func getCacheFile(forWriting bool) *os.File {
	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}

	var openMode int
	if forWriting {
		openMode = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	} else {
		openMode = os.O_RDONLY | os.O_CREATE
	}

	cacheFile, err := os.OpenFile(fmt.Sprintf("%v/bfserver/cache", configDir), openMode, 0644)
	if err != nil {
		panic(err)
	}

	return cacheFile
}

func inSeenCache(needleRunId int64) bool {
	cacheFile := getCacheFile(false)
	defer cacheFile.Close()

	scanner := bufio.NewScanner(cacheFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(strings.TrimSpace(line)) == 0 {
			continue
		}

		if needleRunId == int64(service.MustAtoi(line)) {
			return true
		}
	}
	if scanner.Err() != nil {
		panic(scanner.Err())
	}

	return false
}

func writeSeenCache(runId int64) {
	cacheFile := getCacheFile(true)
	defer cacheFile.Close()

	cacheFile.WriteString(fmt.Sprintf("%v\n", runId))
}

func list() {
	var jiraUsername, jiraToken string
	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}

	secretsFile, err := os.Open(fmt.Sprintf("%v/bfserver/secrets", configDir))
	if err != nil {
		panic(err)
	}
	defer secretsFile.Close()

	scanner := bufio.NewScanner(secretsFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if !found {
			fmt.Println("Bad secrets line:", scanner.Text())
			continue
		}

		switch key {
		case "jira_username":
			jiraUsername = value
		case "jira_api_token":
			jiraToken = value
		}
	}
	tickets := service.GetOpenFlakeyFailureTickets(jiraUsername, jiraToken)

	for _, issue := range tickets {
		desc := issue.Fields.Description
		if len(desc) > 1000 {
			desc = fmt.Sprintf("%v...", desc[:1000])
		}
		fmt.Println("Issue:", issue.Key)
		fmt.Println("Summary:", issue.Fields.Summary)
		fmt.Println("Description:")
		i1 := service.NewIndenterWithPrefix("\t")
		fmt.Println(desc)
		i1.Close()
	}
}

func analyze() {
	args := util.ParseProgramArgs()

	ctx := context.Background()
	client := github.NewTokenClient(ctx, args.GithubToken)

	// Example url: https://github.com/viamrobotics/rdk/actions/runs/5859328480/job/15885094207
	runJobRe := regexp.MustCompile(`/actions/runs/(\d+)/job/(\d+)`)
	matches := runJobRe.FindStringSubmatch(args.Url)

	runId, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		fmt.Println("Error parsing the run id from the link:", args.Url)
		panic(err)
	}
	jobId, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		fmt.Println("Error parsing the job id from the link:", args.Url)
		panic(err)
	}

	var failures []service.Failure
	if args.IsJob {
		failures, err = service.GithubRunToFailedTests(ctx, client, runId, jobId)
	} else if args.IsRun {
		// Passing zero gets failures for all jobs in the run
		allJobs := int64(0)
		failures, err = service.GithubRunToFailedTests(ctx, client, runId, allJobs)
	} else {
		fmt.Println("Must pass --job or --run")
	}
	if err != nil {
		panic(err)
	}

	fmt.Println("Num failures:", len(failures))
	for _, failure := range failures {
		fmt.Printf("%v\n", failure.Variant)
		fmt.Println("---------------------------")
		failure.Output.PrettyPrint("\t")
	}

	fmt.Println("Summary:")
	for _, failure := range failures {
		fmt.Printf("  %v\n", failure.Variant)
		fmt.Println("  ---------------------------")
		failure.Output.ThingsThatFailed("  ", failure.GitHash)
	}

	fmt.Println("Deduping\n---------------------------")
	if args.Dedup {
		fmt.Println("All failures:", failures)
		for _, failure := range failures {
			fmt.Println("Test failures:", failure.Output.TestFailures)
			for _, fqTest := range failure.Output.TestFailures {
				service.RunDedup(failure, fqTest, service.GetOpenFlakeyFailureTickets(args.JiraUsername, args.JiraToken))
			}
		}
	}
}
