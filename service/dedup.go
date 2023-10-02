package service

import (
	"fmt"

	"github.com/andygrunwald/go-jira"
)

func GetSummaryForFailure(runFailure Failure, fqTest FQTest) string {
	artifacts := runFailure.Output
	if assertions := artifacts.Assertions[fqTest]; len(assertions) > 0 {
		return fmt.Sprintf("Test Failure: %v", fqTest)
	} else if timeout := artifacts.Timeouts[fqTest]; timeout != nil {
		return fmt.Sprintf("Test Timeout: %v", fqTest)
	} else if datarace := artifacts.Dataraces[fqTest]; datarace != nil {
		return fmt.Sprintf("Test Datarace: %v", fqTest)
	} else {
		panic("Unknown")
	}
}

func RunDedup(runFailure Failure, fqTest FQTest, openIssues []jira.Issue) {
	summary := GetSummaryForFailure(runFailure, fqTest)

	fmt.Println("Summary:", summary)
	for _, issue := range openIssues {
		if summary == issue.Fields.Summary {
			fmt.Println("\tDedup match:", issue.Key)
		}
	}
}
