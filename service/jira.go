package service

import (
	"fmt"
	"io"
	"strings"

	"github.com/andygrunwald/go-jira"
	"github.com/trivago/tgo/tcontainer"
)

func CreateNewTicketsFromFailure(runFailure Failure, jiraUsername, jiraToken string, fileTickets bool) []*jira.Issue {
	ret := make([]*jira.Issue, 0)

	tp := jira.BasicAuthTransport{
		Username: jiraUsername,
		Password: jiraToken,
	}

	artifacts := runFailure.Output
	jiraClient, _ := jira.NewClient(tp.Client(), "https://viam.atlassian.net/")
	for _, fqTest := range artifacts.TestFailures {
		fmt.Println("Test:", fqTest, "NumLogs:", len(artifacts.Logs[fqTest]))
		if len(artifacts.Logs[fqTest]) == 0 {
			if GDebug {
				fmt.Println("  No logs, skipping:", fqTest)
			}
			continue
		}

		var summary string
		var assertionMsg string
		if assertions := artifacts.Assertions[fqTest]; len(assertions) > 0 {
			summary = fmt.Sprintf("Test Failure: %v", fqTest)
			assertionMsg = assertions[0].ToPrettyString("")
		} else if timeout := artifacts.Timeouts[fqTest]; timeout != nil {
			summary = fmt.Sprintf("Test Timeout: %v", fqTest)
			assertionMsg = timeout.LogLines[0]
		} else if datarace := artifacts.Dataraces[fqTest]; datarace != nil {
			summary = fmt.Sprintf("Test Datarace: %v", fqTest)
			assertionMsg = datarace.LogLines[0]
		}

		ticket := &jira.Issue{
			Fields: &jira.IssueFields{
				Project: jira.Project{
					Key: "RSDK",
				},
				Type: jira.IssueType{
					Name: "Bug",
				},
				Summary: summary,
				Description: fmt.Sprintf("[Github Run|%v]\n\n"+
					"Assertion:\n\n{noformat}\n%v\n{noformat}\n\n"+
					"Logs:\n\n{noformat}\n%v\n{noformat}\n\n",
					runFailure.GithubLink,
					assertionMsg,
					strings.Join(artifacts.Logs[fqTest], "\n")),
				Labels: []string{"flaky_test"},
				Unknowns: tcontainer.MarshalMap(map[string]interface{}{
					"customfield_10074": []map[string]string{
						map[string]string{"value": "Default"},
					},
				}),
			},
		}

		ret = append(ret, ticket)
		if !fileTickets {
			continue
		}

		filed, resp, err := jiraClient.Issue.Create(ticket)
		if err != nil {
			fmt.Println(resp.Header)
			msg, err2 := io.ReadAll(resp.Body)
			fmt.Println(string(msg))
			fmt.Println(err2)
			panic(err)
		}
		ticket.Key = filed.Key
	}

	return ret
}
