package service

import (
	"fmt"

	"github.com/andygrunwald/go-jira"
	"github.com/viamrobotics/bfserver/util"
)

func GetSummaryForFailure(runFailure Failure, fqTest FQTest) (string, error) {
	artifacts := runFailure.Output
	if assertions := artifacts.Assertions[fqTest]; len(assertions) > 0 {
		return fmt.Sprintf("Test Failure: %v", fqTest), nil
	} else if timeout := artifacts.Timeouts[fqTest]; timeout != nil {
		return fmt.Sprintf("Test Timeout: %v", fqTest), nil
	} else if datarace := artifacts.Dataraces[fqTest]; datarace != nil {
		return fmt.Sprintf("Test Datarace: %v", fqTest), nil
	} else {
		if util.GDebug {
			fmt.Println("Failure not found")
			for key := range artifacts.Dataraces {
				fmt.Printf("DataraceKey: `%s`\n", key)
			}
		}

		return "", fmt.Errorf("Unknown: `%s`", fqTest)
	}
}

func RunDedup(runFailure Failure, fqTest FQTest, openIssues []jira.Issue) error {
	summary, err := GetSummaryForFailure(runFailure, fqTest)
	if err != nil {
		return err
	}

	fmt.Println("Summary:", summary)
	for _, issue := range openIssues {
		if summary == issue.Fields.Summary {
			fmt.Println("\tDedup match:", issue.Key)
		}
	}

	return nil
}
