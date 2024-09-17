package service

import (
	"fmt"
	"strings"

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
	} else if runtimeError := artifacts.RuntimeErrors[fqTest]; runtimeError != nil {
		return fmt.Sprintf("Test RuntimeError: %v", fqTest), nil
	} else {
		if util.GDebug {
			fmt.Println("Failure not found:", fqTest)
			for failureFQTest := range artifacts.Dataraces {
				fmt.Printf("DataraceKey: `%s`\n", failureFQTest)
			}
			for failureFQTest := range artifacts.RuntimeErrors {
				fmt.Printf("RuntimeErrorKey: `%s`\n", failureFQTest)
				if strings.HasPrefix(string(fqTest), string(failureFQTest)) {
					fmt.Println("  Match")
					return fmt.Sprintf("Test RuntimeError: %v", fqTest), nil
				}
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
