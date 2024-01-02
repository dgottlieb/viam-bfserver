package service

import (
	"fmt"
	"io"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/trivago/tgo/tcontainer"
	"github.com/viamrobotics/bfserver/util"
)

func truncate(logs []string, maxSize int) string {
	// Join the logs with newlines. Use the first N-th logs that total < `maxSize` bytes.
	totalSize := 0
	for maxIdx := 0; maxIdx < len(logs); maxIdx++ {
		if totalSize+len(logs[maxIdx])+1 > maxSize {
			return fmt.Sprintf("%s\n%s",
				"Test logs truncated for jira filing purposes:",
				strings.Join(logs[:maxIdx], "\n"))
		}

		totalSize += len(logs[maxIdx]) + 1
	}

	return strings.Join(logs, "\n")
}

// `PushTickets` will run dedup logic and either:
// - Create a new ticket for a new failure.
// - Add a link to an existing ticket for a deduped failure.
//
// `newTickets` input is modified in place with the `Issue.Key` value from the jira API response.
func PushTickets(newTickets []*jira.Issue, existingTickets []jira.Issue, githubRunUrl, jiraUsername, jiraToken string) error {
	tp := jira.BasicAuthTransport{
		Username: jiraUsername,
		Password: jiraToken,
	}
	jiraClient, _ := jira.NewClient(tp.Client(), "https://viam.atlassian.net/")

	// For deduping. Returns non-empty ticket string on match. E.g: `RSDK-5192`.
	exists := func(failure *jira.Issue) string {
		for _, existingTicket := range existingTickets {
			if failure.Fields.Summary == existingTicket.Fields.Summary {
				return existingTicket.Key
			}
		}

		return ""
	}

	for _, ticket := range newTickets {
		if name := exists(ticket); name != "" {
			ticket.Key = name
			fmt.Println("Failure exists.\n\tTicket:", name, "\n\tSummary:", ticket.Fields.Summary)
			jiraClient.Issue.AddRemoteLink(name, &jira.RemoteLink{
				Object: &jira.RemoteLinkObject{
					URL:   githubRunUrl,
					Title: "Failure run",
				}})

			continue
		}

		filed, resp, err := jiraClient.Issue.Create(ticket)
		if err != nil {
			fmt.Println("Header:", resp.Header)
			msg, err2 := io.ReadAll(resp.Body)
			fmt.Println("Msg:", string(msg))
			fmt.Println("Reading err?", err2)
			panic(err)
		}
		ticket.Key = filed.Key
	}

	return nil
}

func CreateTicketObjectsFromFailure(runFailure Failure) []*jira.Issue {
	ret := make([]*jira.Issue, 0)

	artifacts := runFailure.Output
	for _, fqTest := range artifacts.TestFailures {
		fmt.Println("Test:", fqTest, "NumLogs:", len(artifacts.Logs[fqTest]))
		if len(artifacts.Logs[fqTest]) == 0 {
			if util.GDebug {
				fmt.Println("  No logs, skipping:", fqTest)
			}
			continue
		}

		var summary string
		var assertionMsg string
		var assertionCodeLink string

		// Consolidate with `GetSummaryForFailure`?
		if assertions := artifacts.Assertions[fqTest]; len(assertions) > 0 {
			summary = fmt.Sprintf("Test Failure: %v", fqTest)
			assertionMsg = assertions[0].ToPrettyString("")
			assertionCodeLink = assertions[0].GetAssertionCodeLinkWithText(
				" (Code Link)", runFailure)
		} else if timeout := artifacts.Timeouts[fqTest]; timeout != nil {
			summary = fmt.Sprintf("Test Timeout: %v", fqTest)
			assertionMsg = timeout.LogLines[0]
		} else if datarace := artifacts.Dataraces[fqTest]; datarace != nil {
			summary = fmt.Sprintf("Test Datarace: %v", fqTest)
			assertionMsg = datarace.LogLines[0]
		} else {
			panic("Unknown")
		}

		var project string
		switch runFailure.WorkflowRun.GetRepository().GetName() {
		case "rdk":
			project = "RSDK"
		case "goutils":
			project = "RSDK"
		case "app":
			project = "APP"
		}

		ticket := &jira.Issue{
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Key: project,
				},
				Type: jira.IssueType{
					Name: "Bug",
				},
				Summary: summary,
				Description: fmt.Sprintf("[Github Run|%v]\n\n"+
					"Assertion%s:\n\n{noformat}\n%v\n{noformat}\n\n"+
					"Logs:\n\n{noformat}\n%v\n{noformat}\n\n",
					runFailure.GithubLink,
					assertionCodeLink,
					assertionMsg,
					// Jira errors if the description is too long:
					// "errors":{
					//   "description":"The entered text is too long. It exceeds the allowed limit of 32,767 characters."
					// }
					truncate(artifacts.Logs[fqTest], 30000)),
				Labels: []string{"flaky_test"},
				Unknowns: tcontainer.MarshalMap(map[string]interface{}{
					"customfield_10074": []map[string]string{
						map[string]string{"value": "Default"},
					},
				}),
			},
		}

		ret = append(ret, ticket)
	}

	return ret
}

func GetOpenFlakeyFailureTickets(jiraUsername, jiraToken string) []jira.Issue {
	tp := jira.BasicAuthTransport{
		Username: jiraUsername,
		Password: jiraToken,
	}

	jiraClient, _ := jira.NewClient(tp.Client(), "https://viam.atlassian.net/")
	const flakeyTestFilterId = 10151
	filter, _, err := jiraClient.Filter.Get(flakeyTestFilterId)
	if err != nil {
		panic(err)
	}

	ret, _, err := jiraClient.Issue.Search(filter.Jql, &jira.SearchOptions{
		StartAt:    0,
		MaxResults: 1000,
		Expand:     "",
	})
	if err != nil {
		panic(err)
	}

	return ret
}
